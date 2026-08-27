package assistant

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
)

const (
	// confirmationTTL is how long the user has to approve a proposed call. It
	// is generous enough to read what is being asked and short enough that an
	// approval never applies to a conversation the user has moved on from.
	confirmationTTL = 10 * time.Minute

	// maxPendingConfirmations bounds the store. Nothing here is durable: a
	// restart drops the pending calls, and the user is asked again.
	maxPendingConfirmations = 256

	// confirmationIDBytes is the size of the identifier. It is unguessable on
	// purpose: it is one half of what authorises the call, the caller's own
	// credential being the other.
	confirmationIDBytes = 16
)

var (
	// ErrUnknownConfirmation is returned for an approval that names a call this
	// instance is not holding: it expired, it was already used, it belongs to
	// somebody else, or the service restarted.
	ErrUnknownConfirmation = errors.New("this action is no longer waiting for approval")

	// ErrTooManyConfirmations is returned when the store is full.
	ErrTooManyConfirmations = errors.New("too many actions are waiting for approval")
)

// ConfirmationRequest is a tool call the assistant may not make on its own.
type ConfirmationRequest struct {
	// ID is what the client sends back to approve this exact call.
	ID string
	// Tool is the name of the tool, Title its human readable name.
	Tool  string
	Title string
	// Arguments is the pretty-printed JSON the tool would be called with, so
	// that the user approves what will actually be sent rather than a summary
	// of it.
	Arguments string
	// Destructive is set when the call removes something.
	Destructive bool
}

// SecretValue is a credential an approved call produced. It goes to the user
// and not to the model: an API token has no business in a prompt sent to an
// external provider.
type SecretValue struct {
	Label string
	Value string
	Note  string
}

// pendingCall is a proposed call waiting for its user.
type pendingCall struct {
	call    ai.ToolCall
	title   string
	owner   string
	expires time.Time
}

// pendingCalls holds the proposed calls of every chat this instance is running.
// It is in memory on purpose: an approval is part of one conversation, and a
// conversation does not outlive the process that started it.
type pendingCalls struct {
	mu    sync.Mutex
	calls map[string]pendingCall
	ttl   time.Duration
	max   int
	// now is the clock, overridable in tests.
	now func() time.Time
}

func newPendingCalls(ttl time.Duration, maximum int) *pendingCalls {
	if ttl <= 0 {
		ttl = confirmationTTL
	}

	if maximum <= 0 {
		maximum = maxPendingConfirmations
	}

	return &pendingCalls{
		calls: make(map[string]pendingCall),
		ttl:   ttl,
		max:   maximum,
		now:   time.Now,
	}
}

// add remembers a proposed call and returns the identifier that approves it.
// The owner is whoever may approve it: an identifier handed to one user must
// not be usable by another.
func (p *pendingCalls) add(call ai.ToolCall, title, owner string) (string, error) {
	id, err := newConfirmationID()
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.pruneLocked()

	if len(p.calls) >= p.max {
		return "", ErrTooManyConfirmations
	}

	p.calls[id] = pendingCall{
		call:    call,
		title:   title,
		owner:   owner,
		expires: p.now().Add(p.ttl),
	}

	return id, nil
}

// take returns the call an approval names and forgets it, so that the same
// approval cannot run the same action twice.
func (p *pendingCalls) take(id, owner string) (ai.ToolCall, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pruneLocked()

	pending, ok := p.calls[id]
	if !ok || pending.owner != owner {
		// The two cases are not told apart: an approval that names somebody
		// else's call must not be distinguishable from one that names nothing.
		return ai.ToolCall{}, "", ErrUnknownConfirmation
	}

	delete(p.calls, id)

	return pending.call, pending.title, nil
}

func (p *pendingCalls) pruneLocked() {
	now := p.now()

	for id, pending := range p.calls {
		if pending.expires.Before(now) {
			delete(p.calls, id)
		}
	}
}

func newConfirmationID() (string, error) {
	buffer := make([]byte, confirmationIDBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(buffer), nil
}

// ownerOf fingerprints the credential a chat runs with. The credential itself
// is never stored: an approval only has to prove that it comes from the same
// session that was asked.
func ownerOf(authorization string) string {
	sum := sha256.Sum256([]byte(authorization))

	return hex.EncodeToString(sum[:])
}

// renderArguments pretty-prints what the tool would be called with. The model
// wrote these arguments, so the user is shown them verbatim rather than a
// description of them.
func renderArguments(arguments json.RawMessage) string {
	if len(arguments) == 0 {
		return "{}"
	}

	var decoded any
	if err := json.Unmarshal(arguments, &decoded); err != nil {
		return string(arguments)
	}

	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return string(arguments)
	}

	return string(encoded)
}

// splitSecret takes a credential out of a tool result. MCP Server puts one in a
// top level "secret" field precisely so that it can be lifted out here: what is
// left goes to the model, and the secret itself goes to the user.
//
// The convention is documented in mcp-server/README.md; a result without such a
// field passes through untouched.
func splitSecret(result string) (string, *SecretValue) {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		return result, nil
	}

	raw, ok := decoded["secret"]
	if !ok {
		return result, nil
	}

	var secret SecretValue
	if err := json.Unmarshal(raw, &struct {
		Label *string `json:"label"`
		Value *string `json:"value"`
		Note  *string `json:"note"`
	}{Label: &secret.Label, Value: &secret.Value, Note: &secret.Note}); err != nil {
		return result, nil
	}

	if secret.Value == "" {
		return result, nil
	}

	decoded["secret"] = json.RawMessage(`"[delivered to the user, not shown here]"`)

	stripped, err := json.Marshal(decoded)
	if err != nil {
		// Failing to re-encode must not leak the secret into the prompt.
		return `{"status":"done"}`, &secret
	}

	return string(stripped), &secret
}
