package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"github.com/runtime-radar/runtime-radar/notifier/api"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/assistant"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/database"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const authorizationMetadataKey = "authorization"

// Roles a client may send in a conversation. Tool traffic is the server's own
// business and is never replayed by a client.
const (
	chatRoleUser      = "user"
	chatRoleAssistant = "assistant"
)

// AssistantGeneric answers chat questions with the model of an AI integration,
// driving the read-only tools of MCP Server.
type AssistantGeneric struct {
	api.UnimplementedAssistantControllerServer

	IntegrationRepository database.IntegrationRepository
	Crypter               cipher.Crypter
	Runner                *assistant.Runner
	// NewClient builds the model client. It is a field so that tests can
	// replace the provider without a live endpoint.
	NewClient func(*model.AI) (ai.Client, error)
}

func (ag *AssistantGeneric) Chat(req *api.ChatReq, stream api.AssistantController_ChatServer) error {
	ctx := stream.Context()

	conversation, err := convertConversation(req.GetConversation())
	if err != nil {
		return err
	}

	eventID, err := validateEventID(req.GetEventId())
	if err != nil {
		return err
	}

	client, err := ag.modelClient(ctx, req.GetIntegrationId())
	if err != nil {
		return err
	}

	runErr := ag.Runner.Run(ctx, assistant.Request{
		Client:        client,
		Conversation:  conversation,
		EventID:       eventID,
		Authorization: authorizationFromContext(ctx),
	}, func(chunk assistant.Chunk) error {
		return stream.Send(convertChunk(chunk))
	})

	switch {
	case runErr == nil:
		return nil

	case errors.Is(runErr, assistant.ErrBusy):
		return status.Error(codes.ResourceExhausted, "the assistant is busy, try again shortly")

	case errors.Is(runErr, assistant.ErrNoUserMessage):
		return status.Error(codes.InvalidArgument, runErr.Error())

	default:
		// Everything the loop itself could not do has already been reported to
		// the client as a done chunk; what is left is the stream being gone.
		log.Debug().Err(runErr).Msg("Assistant chat ended early")

		return nil
	}
}

// modelClient loads the AI integration the caller picked and builds its client.
func (ag *AssistantGeneric) modelClient(ctx context.Context, integrationID string) (ai.Client, error) {
	if integrationID == "" {
		return nil, status.Error(codes.InvalidArgument, "integration ID is empty")
	}

	id, err := uuid.Parse(integrationID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse integration ID: %v", err)
	}

	integration, err := ag.IntegrationRepository.GetByTypeAndID(ctx, model.IntegrationAI, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "integration not found")
		}

		return nil, status.Errorf(codes.Internal, "can't get integration: %v", err)
	}

	aiIntegration, ok := integration.(*model.AI)
	if !ok {
		return nil, status.Errorf(codes.Internal, "invalid integration type given: %T", integration)
	}

	aiIntegration.DecryptSensitive(ag.Crypter)

	newClient := ag.NewClient
	if newClient == nil {
		newClient = ai.NewClient
	}

	client, err := newClient(aiIntegration)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't build ai client: %v", err)
	}

	return client, nil
}

// convertConversation maps the history a client sent onto chat messages.
func convertConversation(messages []*api.ChatMessage) ([]ai.Message, error) {
	converted := make([]ai.Message, 0, len(messages))

	for i, message := range messages {
		var role ai.Role

		switch message.GetRole() {
		case chatRoleUser:
			role = ai.RoleUser
		case chatRoleAssistant:
			role = ai.RoleAssistant
		default:
			return nil, status.Errorf(codes.InvalidArgument, "message %d has unsupported role %q", i, message.GetRole())
		}

		converted = append(converted, ai.Message{Role: role, Content: message.GetContent()})
	}

	return converted, nil
}

// validateEventID checks the event reference before it reaches a prompt. It is
// a UUID or nothing: free text here would be a way to write instructions into
// the assistant's context.
func validateEventID(eventID string) (string, error) {
	if eventID == "" {
		return "", nil
	}

	if _, err := uuid.Parse(eventID); err != nil {
		return "", status.Errorf(codes.InvalidArgument, "can't parse event ID: %v", err)
	}

	return eventID, nil
}

func convertChunk(chunk assistant.Chunk) *api.ChatChunk {
	switch {
	case chunk.Tool != nil:
		return &api.ChatChunk{Chunk: &api.ChatChunk_ToolActivity{ToolActivity: &api.ToolActivity{
			Name:  chunk.Tool.Name,
			Phase: chunk.Tool.Phase,
			Error: chunk.Tool.Error,
		}}}

	case chunk.Done != nil:
		return &api.ChatChunk{Chunk: &api.ChatChunk_Done{Done: &api.Done{
			StopReason: chunk.Done.StopReason,
			Error:      chunk.Done.Error,
			Iterations: uint32(chunk.Done.Iterations), // #nosec G115 -- bounded by the runner's iteration limit
		}}}

	default:
		return &api.ChatChunk{Chunk: &api.ChatChunk_Delta{Delta: chunk.Delta}}
	}
}

// authorizationFromContext returns the caller's own credential, which travels
// on to MCP Server so that the tools run with the caller's permissions.
func authorizationFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(authorizationMetadataKey)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
