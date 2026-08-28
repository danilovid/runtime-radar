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
	//
	// A message is allowed to be large because one may carry attached files:
	// the interface renders them into the question itself. Its own budget for
	// them is smaller than this, so that a message with the most files it
	// allows still fits here alongside what the user typed — see
	// ASSISTANT_MAX_ATTACHMENTS_BYTES in libs/domains/assistant. Exceeding any
	// of these bounds is refused outright rather than trimmed: a question the
	// model answers has to be the question that was asked.
	maxConversationMessages = 50
	maxMessageBytes         = 16 * 1024
	maxConversationBytes    = 64 * 1024

	// Phases reported in a ToolEvent.
	PhaseStarted  = "started"
	PhaseFinished = "finished"

	// Reasons a chat ends, reported in Done.
	StopReasonEndTurn              = "end_turn"
	StopReasonMaxIterations        = "max_iterations"
	StopReasonConfirmationRequired = "confirmation_required"
	StopReasonError                = "error"

	// Short constants for the error metric. An error message may quote user
	// data, so it never becomes a label.
	errorReasonBusy    = "busy"
	errorReasonTools   = "tools_unavailable"
	errorReasonModel   = "model_failed"
	errorReasonInvalid = "invalid_request"
	errorReasonConfirm = "confirmation_unknown"
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
	Delta        string
	Tool         *ToolEvent
	Confirmation *ConfirmationRequest
	Secret       *SecretValue
	Done         *DoneInfo
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
	// Mode selects the instructions the assistant is given for this turn. It
	// never widens what the assistant may do.
	Mode Mode
	// ConfirmID, when set, approves the tool call the assistant proposed in the
	// previous turn. It is the only way a tool that changes anything is run.
	ConfirmID string
	// Scopes are the halves of the product the chosen AI integration allows.
	// Empty offers every tool the caller's role permits.
	Scopes []string

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
	pending       *pendingCalls
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
		pending:       newPendingCalls(confirmationTTL, maxPendingConfirmations),
	}
}

// Run answers one question, emitting the answer as it is produced. It always
// emits a final Done chunk unless the client went away.
func (r *Runner) Run(ctx context.Context, req Request, emit Emit) error {
	// A turn that approves a proposed call carries no new question: the
	// conversation ends with the assistant asking, and the approval itself is
	// the user's answer.
	if err := validateConversation(req.Conversation, req.ConfirmID == ""); err != nil {
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

	tools, iterations, confirming, err := r.run(ctx, req, emit)

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

	switch {
	case confirming:
		stopReason = StopReasonConfirmationRequired
	case iterations >= r.maxIterations:
		stopReason = StopReasonMaxIterations
	}

	metrics.ObserveAssistantChat(stopReason, iterations, time.Since(started))

	return emit(Chunk{Done: &DoneInfo{StopReason: stopReason, Iterations: iterations}})
}

// errEmitFailed marks the client going away, which is not a failure of the run.
var errEmitFailed = errors.New("can't emit chunk")

// run is the agent loop. It returns the metric reason of a failure alongside
// the error, so that Run can count it without parsing the message.
func (r *Runner) run(ctx context.Context, req Request, emit Emit) (errorReason string, iterations int, confirming bool, err error) {
	box, err := r.tools.Open(ctx, req.Authorization, req.Scopes)
	if err != nil {
		return errorReasonTools, 0, false, fmt.Errorf("can't reach the assistant tools: %w", err)
	}
	defer func() {
		if closeErr := box.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("Can't close MCP session")
		}
	}()

	tools, err := box.ListTools(ctx)
	if err != nil {
		return errorReasonTools, 0, false, fmt.Errorf("can't list the assistant tools: %w", err)
	}

	byName := make(map[string]ai.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	messages := buildMessages(req)

	// An approval runs the very call the user agreed to, before the model gets
	// another turn: the model cannot change the arguments after the fact.
	if req.ConfirmID != "" {
		message, err := r.runApproved(ctx, box, req, emit)

		switch {
		case errors.Is(err, errEmitFailed):
			return "", 0, false, errEmitFailed
		case err != nil:
			return errorReasonConfirm, 0, false, err
		}

		messages = append(messages, message)
	}

	for iterations = 0; iterations < r.maxIterations; iterations++ {
		// The answer is forwarded to the client as the model writes it. An emit
		// that fails means the client is gone; the loop notices right after the
		// call rather than in the middle of a callback.
		var emitErr error

		result, err := req.Client.Chat(ctx, messages, tools, func(delta string) {
			if emitErr != nil || delta == "" {
				return
			}

			emitErr = emit(Chunk{Delta: delta})
		})

		if emitErr != nil {
			return "", iterations, false, errEmitFailed
		}

		if err != nil {
			return errorReasonModel, iterations, false, fmt.Errorf("can't get an answer from the model: %w", err)
		}

		if len(result.ToolCalls) == 0 {
			return "", iterations + 1, false, nil
		}

		messages = append(messages, ai.Message{
			Role:      ai.RoleAssistant,
			Content:   result.Text,
			ToolCalls: result.ToolCalls,
		})

		for _, call := range result.ToolCalls {
			// A tool that changes the product ends the turn: the user is shown
			// the call and answers it themselves. Anything the model asked for
			// alongside it is dropped, and it may ask again once the answer is
			// in — a half-applied turn is not something to keep.
			if tool, ok := byName[call.Name]; ok && !tool.ReadOnly {
				err := r.askForConfirmation(req, call, tool, emit)

				switch {
				case errors.Is(err, errEmitFailed):
					return "", iterations, false, errEmitFailed
				case err != nil:
					return errorReasonConfirm, iterations, false, err
				}

				return "", iterations + 1, true, nil
			}

			message, emitErr := r.runTool(ctx, box, call, emit)
			if emitErr != nil {
				return "", iterations, false, errEmitFailed
			}

			messages = append(messages, message)
		}
	}

	// Out of iterations with the model still asking for tools: say so, rather
	// than leaving the user with a half-finished thought.
	if emitErr := emit(Chunk{Delta: maxIterationsPrompt}); emitErr != nil {
		return "", iterations, false, errEmitFailed
	}

	return "", iterations, false, nil
}

// askForConfirmation remembers the proposed call and asks the user about it.
// Nothing is run here: the call waits until the user sends its identifier back.
func (r *Runner) askForConfirmation(req Request, call ai.ToolCall, tool ai.Tool, emit Emit) error {
	id, err := r.pending.add(call, tool.Title, ownerOf(req.Authorization))
	if err != nil {
		return err
	}

	log.Info().Str("tool", call.Name).Msg("Assistant is asking the user to approve a tool call")

	if emitErr := emit(Chunk{Confirmation: &ConfirmationRequest{
		ID:          id,
		Tool:        call.Name,
		Title:       tool.Title,
		Arguments:   renderArguments(call.Arguments),
		Destructive: tool.Destructive,
	}}); emitErr != nil {
		return errEmitFailed
	}

	return nil
}

// runApproved runs the call the user agreed to and returns what the model is
// told about it. A credential the call produced is taken out on the way: it
// goes to the user and never into a prompt.
func (r *Runner) runApproved(ctx context.Context, box ToolBox, req Request, emit Emit) (ai.Message, error) {
	call, title, err := r.pending.take(req.ConfirmID, ownerOf(req.Authorization))
	if err != nil {
		return ai.Message{}, err
	}

	if emitErr := emit(Chunk{Tool: &ToolEvent{Name: call.Name, Phase: PhaseStarted}}); emitErr != nil {
		return ai.Message{}, errEmitFailed
	}

	started := time.Now()
	result, callErr := box.CallTool(ctx, call.Name, call.Arguments)
	metrics.ObserveAssistantToolCall(call.Name, time.Since(started), callErr)
	metrics.ObserveAssistantConfirmedCall(call.Name, callErr)

	log.Info().Str("tool", call.Name).Str("title", title).Err(callErr).Msg("Assistant ran a tool the user approved")

	event := &ToolEvent{Name: call.Name, Phase: PhaseFinished}

	if callErr != nil {
		event.Error = callErr.Error()
	} else {
		var secret *SecretValue

		result, secret = splitSecret(result)

		if secret != nil {
			if emitErr := emit(Chunk{Secret: secret}); emitErr != nil {
				return ai.Message{}, errEmitFailed
			}
		}
	}

	if emitErr := emit(Chunk{Tool: event}); emitErr != nil {
		return ai.Message{}, errEmitFailed
	}

	return ai.Message{Role: ai.RoleUser, Content: approvedCallPrompt(call, result, callErr)}, nil
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
// instructions of the mode this turn runs in, the event the chat was opened
// from, and the conversation the client sent.
func buildMessages(req Request) []ai.Message {
	messages := make([]ai.Message, 0, len(req.Conversation)+3)
	messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: systemPrompt})

	if prompt := req.Mode.prompt(); prompt != "" {
		messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: prompt})
	}

	if req.EventID != "" {
		messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: eventContextPrompt(req.EventID)})
	}

	return append(messages, req.Conversation...)
}

// validateConversation bounds what a client may send. The history is replayed
// by the client on every request, so its size is the client's choice until it
// is bounded here.
func validateConversation(conversation []ai.Message, mustEndWithUser bool) error {
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

	if mustEndWithUser && conversation[len(conversation)-1].Role != ai.RoleUser {
		return ErrNoUserMessage
	}

	return nil
}
