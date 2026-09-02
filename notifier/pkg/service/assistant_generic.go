package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
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
// driving the tools of MCP Server. The tools that change the product are only
// run once the client sends back the approval identifier the assistant asked
// for, which is what confirm_id carries.
type AssistantGeneric struct {
	api.UnimplementedAssistantControllerServer

	IntegrationRepository database.IntegrationRepository
	// ChatRepository stores the conversations. It may be nil, and the assistant
	// then answers exactly as before without keeping anything.
	ChatRepository database.ChatRepository
	Crypter        cipher.Crypter
	Runner         *assistant.Runner
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

	client, scopes, err := ag.modelClient(ctx, req.GetIntegrationId())
	if err != nil {
		return err
	}

	confirmID, err := validateConfirmID(req.GetConfirmId())
	if err != nil {
		return err
	}

	// The conversation is stored as it happens rather than handed over by the
	// client afterwards: what is kept is then what the assistant actually said,
	// and a client that never comes back still leaves a readable record.
	chatID, storeErr := ag.openChat(ctx, req, conversation)
	if storeErr != nil {
		log.Warn().Err(storeErr).Msg("Can't store the chat, answering without keeping it")
	}

	answer := &strings.Builder{}

	runErr := ag.Runner.Run(ctx, assistant.Request{
		Client:        client,
		Conversation:  conversation,
		EventID:       eventID,
		EventKind:     assistant.ParseEventKind(req.GetEventKind()),
		Mode:          assistant.ParseMode(req.GetMode()),
		ConfirmID:     confirmID,
		Authorization: authorizationFromContext(ctx),
		Scopes:        scopes,
	}, func(chunk assistant.Chunk) error {
		if chunk.Delta != "" {
			answer.WriteString(chunk.Delta)
		}

		if chunk.Done != nil {
			ag.closeChat(ctx, chatID, answer.String())
		}

		return stream.Send(convertChunk(chunk, chatID))
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

// modelClient builds the client for the chosen integration and returns the
// scopes it limits the assistant to. An empty list means every tool the
// signed-in user's role allows.
func (ag *AssistantGeneric) modelClient(ctx context.Context, integrationID string) (ai.Client, []string, error) {
	if integrationID == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "integration ID is empty")
	}

	id, err := uuid.Parse(integrationID)
	if err != nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "can't parse integration ID: %v", err)
	}

	integration, err := ag.IntegrationRepository.GetByTypeAndID(ctx, model.IntegrationAI, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, status.Error(codes.NotFound, "integration not found")
		}

		return nil, nil, status.Errorf(codes.Internal, "can't get integration: %v", err)
	}

	aiIntegration, ok := integration.(*model.AI)
	if !ok {
		return nil, nil, status.Errorf(codes.Internal, "invalid integration type given: %T", integration)
	}

	aiIntegration.DecryptSensitive(ag.Crypter)

	newClient := ag.NewClient
	if newClient == nil {
		newClient = ai.NewClient
	}

	client, err := newClient(aiIntegration)
	if err != nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "can't build ai client: %v", err)
	}

	return client, aiIntegration.Scopes, nil
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

// validateConfirmID checks the approval identifier before it is looked up. The
// runner issues hex identifiers, so anything else is a client error rather than
// a lookup that would fail anyway.
func validateConfirmID(confirmID string) (string, error) {
	if confirmID == "" {
		return "", nil
	}

	if _, err := hex.DecodeString(confirmID); err != nil {
		return "", status.Error(codes.InvalidArgument, "can't parse confirmation ID")
	}

	return confirmID, nil
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

func convertChunk(chunk assistant.Chunk, chatID string) *api.ChatChunk {
	switch {
	case len(chunk.Suggestions) != 0:
		return &api.ChatChunk{Chunk: &api.ChatChunk_Suggestions{
			Suggestions: &api.Suggestions{Questions: chunk.Suggestions},
		}}

	case chunk.Confirmation != nil:
		return &api.ChatChunk{Chunk: &api.ChatChunk_Confirmation{Confirmation: &api.Confirmation{
			Id:          chunk.Confirmation.ID,
			Tool:        chunk.Confirmation.Tool,
			Title:       chunk.Confirmation.Title,
			Arguments:   chunk.Confirmation.Arguments,
			Destructive: chunk.Confirmation.Destructive,
		}}}

	case chunk.Secret != nil:
		return &api.ChatChunk{Chunk: &api.ChatChunk_Secret{Secret: &api.Secret{
			Label: chunk.Secret.Label,
			Value: chunk.Secret.Value,
			Note:  chunk.Secret.Note,
		}}}

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
			ChatId:     chatID,
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

// maxStoredChats bounds a listing, and titleLimit how much of the first
// question is kept as a name for it.
const (
	maxStoredChats = 100
	titleLimit     = 120
)

// openChat resolves the conversation this turn belongs to, creating it when the
// client has none yet, and writes the user's turn into it. Storage never blocks
// answering: a failure here is logged and the assistant still replies, because
// losing a record is better than losing the answer.
func (ag *AssistantGeneric) openChat(
	ctx context.Context, req *api.ChatReq, conversation []ai.Message,
) (string, error) {
	if ag.ChatRepository == nil {
		return "", nil
	}

	userID, err := callerID(ctx)
	if err != nil {
		return "", err
	}

	question := lastUserMessage(conversation)

	chatID, err := uuid.Parse(req.GetChatId())
	if err != nil {
		chat := &model.Chat{
			UserID:    userID,
			Title:     truncateTitle(question),
			EventID:   req.GetEventId(),
			EventKind: req.GetEventKind(),
			Mode:      req.GetMode(),
		}

		if err := ag.ChatRepository.Add(ctx, chat); err != nil {
			return "", fmt.Errorf("can't create chat: %w", err)
		}

		chatID = chat.ID
	}

	if question == "" {
		return chatID.String(), nil
	}

	messages := []*model.ChatMessage{{Role: model.ChatRoleUser, Content: question}}
	if err := ag.ChatRepository.AppendMessages(ctx, userID, chatID, messages); err != nil {
		return chatID.String(), fmt.Errorf("can't store the question: %w", err)
	}

	return chatID.String(), nil
}

// closeChat writes the answer the assistant gave. An empty answer is not
// stored: a turn that produced only tool activity or an error has nothing to
// show, and a blank message in the history would only confuse the next read.
func (ag *AssistantGeneric) closeChat(ctx context.Context, chatID, answer string) {
	if ag.ChatRepository == nil || chatID == "" || strings.TrimSpace(answer) == "" {
		return
	}

	userID, err := callerID(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Can't store the answer")

		return
	}

	id, err := uuid.Parse(chatID)
	if err != nil {
		return
	}

	messages := []*model.ChatMessage{{Role: model.ChatRoleAssistant, Content: answer}}
	if err := ag.ChatRepository.AppendMessages(ctx, userID, id, messages); err != nil {
		log.Warn().Err(err).Msg("Can't store the answer")
	}
}

func (ag *AssistantGeneric) ListChats(ctx context.Context, req *api.ListChatsReq) (*api.ListChatsResp, error) {
	if ag.ChatRepository == nil {
		return &api.ListChatsResp{}, nil
	}

	userID, err := callerID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > maxStoredChats {
		limit = maxStoredChats
	}

	chats, err := ag.ChatRepository.GetAll(ctx, userID, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't list chats: %v", err)
	}

	resp := &api.ListChatsResp{Chats: make([]*api.Chat, 0, len(chats))}
	for _, chat := range chats {
		resp.Chats = append(resp.Chats, chatToPB(chat, false))
	}

	return resp, nil
}

func (ag *AssistantGeneric) ReadChat(ctx context.Context, req *api.ReadChatReq) (*api.ReadChatResp, error) {
	if ag.ChatRepository == nil {
		return nil, status.Error(codes.NotFound, "chat not found")
	}

	userID, err := callerID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse chat ID: %v", err)
	}

	chat, err := ag.ChatRepository.GetByID(ctx, userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "chat not found")
		}

		return nil, status.Errorf(codes.Internal, "can't get chat: %v", err)
	}

	return &api.ReadChatResp{Chat: chatToPB(chat, true)}, nil
}

func (ag *AssistantGeneric) DeleteChat(ctx context.Context, req *api.DeleteChatReq) (*api.DeleteChatResp, error) {
	if ag.ChatRepository == nil {
		return nil, status.Error(codes.NotFound, "chat not found")
	}

	userID, err := callerID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse chat ID: %v", err)
	}

	if err := ag.ChatRepository.DeleteByID(ctx, userID, id); err != nil {
		return nil, status.Errorf(codes.Internal, "can't delete chat: %v", err)
	}

	return &api.DeleteChatResp{Id: req.GetId()}, nil
}

// callerID is who the conversation belongs to. It comes from the token the
// caller presented, which the auth layer has already verified.
func callerID(ctx context.Context) (string, error) {
	token, err := jwt.UnverifiedTokenFromContext(ctx)
	if err != nil {
		return "", err
	}

	userID := token.GetUserID()
	if userID == "" {
		return "", errors.New("token carries no user")
	}

	return userID, nil
}

// lastUserMessage is the question this turn is about: the client sends the
// whole conversation, but only its final turn is new.
func lastUserMessage(conversation []ai.Message) string {
	for i := len(conversation) - 1; i >= 0; i-- {
		if conversation[i].Role == ai.RoleUser {
			return conversation[i].Content
		}
	}

	return ""
}

func truncateTitle(question string) string {
	title := strings.TrimSpace(question)
	if len([]rune(title)) <= titleLimit {
		return title
	}

	return string([]rune(title)[:titleLimit])
}

func chatToPB(chat *model.Chat, withMessages bool) *api.Chat {
	out := &api.Chat{
		Id:           chat.ID.String(),
		Title:        chat.Title,
		CreatedAt:    chat.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:    chat.UpdatedAt.Format(time.RFC3339Nano),
		EventId:      chat.EventID,
		EventKind:    chat.EventKind,
		Mode:         chat.Mode,
		MessageCount: uint32(messageCount(chat)), // #nosec G115 -- a conversation is not that long
	}

	if !withMessages {
		return out
	}

	out.Messages = make([]*api.ChatMessage, 0, len(chat.Messages))
	for _, message := range chat.Messages {
		out.Messages = append(out.Messages, &api.ChatMessage{Role: message.Role, Content: message.Content})
	}

	return out
}

// messageCount prefers the turns that were loaded, and falls back to the count
// a listing filled in without them.
func messageCount(chat *model.Chat) int {
	if len(chat.Messages) != 0 {
		return len(chat.Messages)
	}

	return chat.MessageCount
}
