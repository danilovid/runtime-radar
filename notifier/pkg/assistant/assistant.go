package assistant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/metrics"
)

const (
	// DefaultMaxIterations bounds the agent loop: every iteration is one model
	// turn, and a turn may run several tools. A question that isn't answered in
	// eight turns is one the user should narrow down instead.
	DefaultMaxIterations = 8

	// DefaultTimeout bounds the whole loop, model calls and tools included.
	DefaultTimeout = 120 * time.Second

	// DefaultMaxConcurrentChats is how many chats one instance runs at once.
	// Each holds a model connection and an MCP session for up to DefaultTimeout.
	DefaultMaxConcurrentChats = 4

	// Bounds on the conversation a client may send. The server keeps no chat
	// state, so the history arrives with every request and has to be bounded
	// here rather than by how much was stored.
	maxConversationMessages = 50
	maxMessageBytes         = 8 * 1024
	maxConversationBytes    = 64 * 1024

	// Phases reported in a ToolEvent.
	PhaseStarted  = "started"
	PhaseFinished = "finished"

	// Reasons a chat ends, reported in Done.
	StopReasonEndTurn       = "end_turn"
	StopReasonMaxIterations = "max_iterations"
	StopReasonError         = "error"

	// Short constants for the error metric. An error message may quote user
	// data, so it never becomes a label.
	errorReasonBusy    = "busy"
	errorReasonTools   = "tools_unavailable"
	errorReasonModel   = "model_failed"
	errorReasonInvalid = "invalid_request"
)

// ErrBusy is returned when the instance already runs as many chats as it may.
var ErrBusy = errors.New("too many concurrent assistant chats")

// ErrNoUserMessage is returned for a conversation that ends with anything but
// a user's message: there would be nothing to answer.
var ErrNoUserMessage = errors.New("conversation must end with a user message")

// ToolEvent reports that the assistant started or finished running a tool.
type ToolEvent struct {
	Name  string
	Phase string
	Error string
}

// DoneInfo ends an answer.
type DoneInfo struct {
	StopReason string
	Error      string
	Iterations int
}

// Chunk is one piece of an answer. Exactly one field is set.
type Chunk struct {
	Delta string
	Tool  *ToolEvent
	Done  *DoneInfo
}

// Emit delivers a chunk to the client. Returning an error stops the run: it
// means the client is gone.
type Emit func(Chunk) error

// Request is one question to answer.
type Request struct {
	// Client is the model of the AI integration the user picked.
	Client ai.Client
	// Conversation is the history the client sent, oldest first, ending with
	// the user's new message.
	Conversation []ai.Message
	// EventID, when set, is the runtime event the question is about.
	EventID string
	// Authorization is the caller's own credential, passed to MCP Server so
	// that the tools run with the caller's permissions.
	Authorization string
}

// Runner executes assistant chats. It is safe for concurrent use.
type Runner struct {
	tools         ToolBoxFactory
	maxIterations int
	timeout       time.Duration
	slots         chan struct{}
}

// NewRunner builds a Runner. Zero values fall back to the defaults above.
func NewRunner(tools ToolBoxFactory, maxIterations int, timeout time.Duration, maxConcurrent int) *Runner {
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrentChats
	}

	return &Runner{
		tools:         tools,
		maxIterations: maxIterations,
		timeout:       timeout,
		slots:         make(chan struct{}, maxConcurrent),
	}
}

// Run answers one question, emitting the answer as it is produced. It always
// emits a final Done chunk unless the client went away.
func (r *Runner) Run(ctx context.Context, req Request, emit Emit) error {
	if err := validateConversation(req.Conversation); err != nil {
		metrics.ObserveAssistantError(errorReasonInvalid)

		return err
	}

	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		metrics.ObserveAssistantError(errorReasonBusy)

		return ErrBusy
	}

	metrics.AssistantChatStarted()
	defer metrics.AssistantChatFinished()

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	started := time.Now()

	tools, iterations, err := r.run(ctx, req, emit)

	// A client that disconnected can't be told anything, so the run just ends.
	if errors.Is(err, errEmitFailed) {
		return err
	}

	if err != nil {
		metrics.ObserveAssistantError(tools)
		metrics.ObserveAssistantChat(StopReasonError, iterations, time.Since(started))

		log.Warn().Err(err).Int("iterations", iterations).Msg("Assistant chat failed")

		return emit(Chunk{Done: &DoneInfo{
			StopReason: StopReasonError,
			Error:      err.Error(),
			Iterations: iterations,
		}})
	}

	stopReason := StopReasonEndTurn
	if iterations >= r.maxIterations {
		stopReason = StopReasonMaxIterations
	}

	metrics.ObserveAssistantChat(stopReason, iterations, time.Since(started))

	return emit(Chunk{Done: &DoneInfo{StopReason: stopReason, Iterations: iterations}})
}

// errEmitFailed marks the client going away, which is not a failure of the run.
var errEmitFailed = errors.New("can't emit chunk")

// run is the agent loop. It returns the metric reason of a failure alongside
// the error, so that Run can count it without parsing the message.
func (r *Runner) run(ctx context.Context, req Request, emit Emit) (errorReason string, iterations int, err error) {
	box, err := r.tools.Open(ctx, req.Authorization)
	if err != nil {
		return errorReasonTools, 0, fmt.Errorf("can't reach the assistant tools: %w", err)
	}
	defer func() {
		if closeErr := box.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("Can't close MCP session")
		}
	}()

	tools, err := box.ListTools(ctx)
	if err != nil {
		return errorReasonTools, 0, fmt.Errorf("can't list the assistant tools: %w", err)
	}

	messages := buildMessages(req)

	for iterations = 0; iterations < r.maxIterations; iterations++ {
		result, err := req.Client.Chat(ctx, messages, tools)
		if err != nil {
			return errorReasonModel, iterations, fmt.Errorf("can't get an answer from the model: %w", err)
		}

		if result.Text != "" {
			if emitErr := emit(Chunk{Delta: result.Text}); emitErr != nil {
				return "", iterations, errEmitFailed
			}
		}

		if len(result.ToolCalls) == 0 {
			return "", iterations + 1, nil
		}

		messages = append(messages, ai.Message{
			Role:      ai.RoleAssistant,
			Content:   result.Text,
			ToolCalls: result.ToolCalls,
		})

		for _, call := range result.ToolCalls {
			message, emitErr := r.runTool(ctx, box, call, emit)
			if emitErr != nil {
				return "", iterations, errEmitFailed
			}

			messages = append(messages, message)
		}
	}

	// Out of iterations with the model still asking for tools: say so, rather
	// than leaving the user with a half-finished thought.
	if emitErr := emit(Chunk{Delta: maxIterationsPrompt}); emitErr != nil {
		return "", iterations, errEmitFailed
	}

	return "", iterations, nil
}

// runTool runs one tool call and returns the message carrying its result back
// to the model. A tool that fails is reported to the model as a failed tool
// rather than ending the run: the model has to account for what it could not
// look up. The error return means the client is gone.
//
// Every tool MCP Server offers today only reads, so every call runs without
// asking. A tool that changes anything must not simply be added here: it needs
// a confirmation round trip — emit a chunk describing the intended change, wait
// for the user to approve it, and only then call the tool — plus a way for this
// loop to tell read tools from write ones. That is deliberately not built yet.
func (r *Runner) runTool(ctx context.Context, box ToolBox, call ai.ToolCall, emit Emit) (ai.Message, error) {
	if err := emit(Chunk{Tool: &ToolEvent{Name: call.Name, Phase: PhaseStarted}}); err != nil {
		return ai.Message{}, err
	}

	started := time.Now()
	result, err := box.CallTool(ctx, call.Name, call.Arguments)
	metrics.ObserveAssistantToolCall(call.Name, time.Since(started), err)

	event := &ToolEvent{Name: call.Name, Phase: PhaseFinished}
	content := result

	if err != nil {
		log.Debug().Err(err).Str("tool", call.Name).Msg("Assistant tool call failed")

		event.Error = err.Error()
		content = "The tool failed: " + err.Error()
	}

	if emitErr := emit(Chunk{Tool: event}); emitErr != nil {
		return ai.Message{}, emitErr
	}

	return ai.Message{
		Role:       ai.RoleTool,
		Content:    content,
		ToolCallID: call.ID,
		ToolName:   call.Name,
	}, nil
}

// buildMessages assembles what the model sees: the assistant's own prompt, the
// event the chat was opened from, and the conversation the client sent.
func buildMessages(req Request) []ai.Message {
	messages := make([]ai.Message, 0, len(req.Conversation)+2)
	messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: systemPrompt})

	if req.EventID != "" {
		messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: eventContextPrompt(req.EventID)})
	}

	return append(messages, req.Conversation...)
}

// validateConversation bounds what a client may send. The history is replayed
// by the client on every request, so its size is the client's choice until it
// is bounded here.
func validateConversation(conversation []ai.Message) error {
	if len(conversation) == 0 {
		return ErrNoUserMessage
	}

	if len(conversation) > maxConversationMessages {
		return fmt.Errorf("conversation is longer than %d messages", maxConversationMessages)
	}

	total := 0
	for i, message := range conversation {
		switch message.Role {
		case ai.RoleUser, ai.RoleAssistant:
		default:
			// Tool traffic belongs to the server side of the loop. Accepting it
			// from a client would let one forge tool results.
			return fmt.Errorf("message %d has unsupported role %q", i, message.Role)
		}

		if len(message.Content) > maxMessageBytes {
			return fmt.Errorf("message %d is longer than %d bytes", i, maxMessageBytes)
		}

		total += len(message.Content)
	}

	if total > maxConversationBytes {
		return fmt.Errorf("conversation is larger than %d bytes", maxConversationBytes)
	}

	if conversation[len(conversation)-1].Role != ai.RoleUser {
		return ErrNoUserMessage
	}

	return nil
}
