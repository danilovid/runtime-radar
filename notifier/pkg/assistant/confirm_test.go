package assistant

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
)

// toolCreateRule is the write tool these tests offer.
const toolCreateRule = "create_rule"

// createRuleArguments is what the model proposes in these tests.
const createRuleArguments = `{"name":"Block cryptominers","block_severity":"high"}`

// testSecret stands in for a credential a tool produced.
const testSecret = "s3cr3t"

// writeToolBox offers one read tool and one that changes the product.
func writeToolBox() *fakeToolBox {
	box := searchDocsBox()
	box.tools = append(box.tools, ai.Tool{
		Name:        toolCreateRule,
		Description: "Create a policy rule.",
		InputSchema: map[string]any{"type": "object"},
		Title:       "Create a policy rule",
		ReadOnly:    false,
	})
	box.results[toolCreateRule] = `{"id":"rule-1","name":"Block cryptominers"}`

	return box
}

// TestRunAsksBeforeChangingAnything is the rule the whole feature rests on: a
// tool that changes the product is never run on the model's word alone.
func TestRunAsksBeforeChangingAnything(t *testing.T) {
	t.Parallel()

	model := &fakeModel{answers: []*ai.ChatResult{
		{
			Text: "I will create the rule.",
			ToolCalls: []ai.ToolCall{{
				ID:        "call_1",
				Name:      toolCreateRule,
				Arguments: json.RawMessage(createRuleArguments),
			}},
		},
	}}

	box := writeToolBox()
	runner := NewRunner(box, DefaultMaxIterations, DefaultTimeout, DefaultMaxConcurrentChats)

	chunks, err := collect(t, runner, Request{
		Client:        model,
		Conversation:  userAsks("создай правило, которое блокирует майнеры"),
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(box.callNames) != 0 {
		t.Fatalf("the tool ran without approval: %v", box.callNames)
	}

	var confirmation *ConfirmationRequest
	for _, chunk := range chunks {
		if chunk.Confirmation != nil {
			confirmation = chunk.Confirmation
		}
	}

	if confirmation == nil {
		t.Fatal("the user was not asked to approve the call")
	}
	if confirmation.Tool != toolCreateRule || confirmation.Title != "Create a policy rule" {
		t.Errorf("confirmation = %+v, want the tool named", confirmation)
	}
	// The user approves the exact arguments the model wrote, not a summary.
	if !strings.Contains(confirmation.Arguments, "Block cryptominers") {
		t.Errorf("confirmation arguments = %q", confirmation.Arguments)
	}
	if confirmation.ID == "" {
		t.Error("the confirmation carries no identifier to answer with")
	}

	last := chunks[len(chunks)-1]
	if last.Done == nil || last.Done.StopReason != StopReasonConfirmationRequired {
		t.Errorf("last chunk = %+v, want the answer waiting for approval", last)
	}
}

// TestRunExecutesApprovedCall covers the other half: the approval runs the very
// call that was proposed, and the model is told what came of it.
func TestRunExecutesApprovedCall(t *testing.T) {
	t.Parallel()

	propose := &fakeModel{answers: []*ai.ChatResult{{
		ToolCalls: []ai.ToolCall{{
			ID:        "call_1",
			Name:      toolCreateRule,
			Arguments: json.RawMessage(createRuleArguments),
		}},
	}}}

	box := writeToolBox()
	runner := NewRunner(box, DefaultMaxIterations, DefaultTimeout, DefaultMaxConcurrentChats)

	chunks, err := collect(t, runner, Request{
		Client:        propose,
		Conversation:  userAsks("создай правило, которое блокирует майнеры"),
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	id := confirmationID(t, chunks)

	report := &fakeModel{answers: []*ai.ChatResult{{Text: "Правило создано."}}}

	if _, err := collect(t, runner, Request{
		Client:        report,
		Conversation:  userAsks("создай правило, которое блокирует майнеры"),
		ConfirmID:     id,
		Authorization: "Bearer user-token",
	}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if len(box.callNames) != 1 || box.callNames[0] != toolCreateRule {
		t.Fatalf("tool calls = %v, want the approved one", box.callNames)
	}
	if box.callArgs[0] != createRuleArguments {
		t.Errorf("tool arguments = %s, want the ones the user approved", box.callArgs[0])
	}

	// The model has no record of asking, so it is told what happened as data.
	conversation := report.conversation(0)
	outcome := conversation[len(conversation)-1]
	if outcome.Role != ai.RoleUser || !strings.Contains(outcome.Content, "<action_result>") {
		t.Errorf("last message = %+v, want the outcome of the approved call", outcome)
	}
	if !strings.Contains(outcome.Content, "rule-1") {
		t.Errorf("the model was not told what the tool returned: %q", outcome.Content)
	}

	// One approval, one call: replaying it must not create a second rule.
	if _, err := collect(t, runner, Request{
		Client:        report,
		Conversation:  userAsks("создай правило"),
		ConfirmID:     id,
		Authorization: "Bearer user-token",
	}); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if len(box.callNames) != 1 {
		t.Errorf("tool calls = %v, want the replayed approval to have been refused", box.callNames)
	}
}

// TestApprovalBelongsToItsUser guards the case that makes an identifier alone
// insufficient: somebody else's approval must not run the call.
func TestApprovalBelongsToItsUser(t *testing.T) {
	t.Parallel()

	propose := &fakeModel{answers: []*ai.ChatResult{{
		ToolCalls: []ai.ToolCall{{ID: "call_1", Name: toolCreateRule, Arguments: json.RawMessage(createRuleArguments)}},
	}}}

	box := writeToolBox()
	runner := NewRunner(box, DefaultMaxIterations, DefaultTimeout, DefaultMaxConcurrentChats)

	chunks, err := collect(t, runner, Request{
		Client:        propose,
		Conversation:  userAsks("создай правило"),
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	id := confirmationID(t, chunks)

	answered, err := collect(t, runner, Request{
		Client:        &fakeModel{answers: []*ai.ChatResult{{Text: "..."}}},
		Conversation:  userAsks("создай правило"),
		ConfirmID:     id,
		Authorization: "Bearer another-user-token",
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if len(box.callNames) != 0 {
		t.Errorf("tool calls = %v, want none for a stranger's approval", box.callNames)
	}

	last := answered[len(answered)-1]
	if last.Done == nil || last.Done.StopReason != StopReasonError {
		t.Errorf("last chunk = %+v, want the approval refused", last)
	}
}

// TestApprovedCallSecretGoesToTheUser checks that a credential a tool produced
// reaches the user and not the model: an API token has no business being sent
// to an LLM provider.
func TestApprovedCallSecretGoesToTheUser(t *testing.T) {
	t.Parallel()

	const toolCreateToken = "create_api_token"

	box := searchDocsBox()
	box.tools = append(box.tools, ai.Tool{
		Name:        toolCreateToken,
		InputSchema: map[string]any{"type": "object"},
		Title:       "Create an API token",
	})
	box.results[toolCreateToken] = `{"id":"token-1","name":"ci-bot","secret":{"label":"API token ci-bot","value":"` + testSecret + `","note":"shown once"}}`

	runner := NewRunner(box, DefaultMaxIterations, DefaultTimeout, DefaultMaxConcurrentChats)

	propose := &fakeModel{answers: []*ai.ChatResult{{
		ToolCalls: []ai.ToolCall{{ID: "call_1", Name: toolCreateToken, Arguments: json.RawMessage(`{"name":"ci-bot"}`)}},
	}}}

	chunks, err := collect(t, runner, Request{
		Client:        propose,
		Conversation:  userAsks("выпусти токен для CI"),
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	report := &fakeModel{answers: []*ai.ChatResult{{Text: "Токен создан."}}}

	confirmed, err := collect(t, runner, Request{
		Client:        report,
		Conversation:  userAsks("выпусти токен для CI"),
		ConfirmID:     confirmationID(t, chunks),
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	var secret *SecretValue
	for _, chunk := range confirmed {
		if chunk.Secret != nil {
			secret = chunk.Secret
		}
	}

	if secret == nil || secret.Value != testSecret {
		t.Fatalf("secret = %+v, want the token delivered to the user", secret)
	}

	for _, message := range report.conversation(0) {
		if strings.Contains(message.Content, testSecret) {
			t.Fatalf("the secret reached the model: %q", message.Content)
		}
	}
}

// TestUnknownApproval covers the ordinary case of an approval that expired or
// belongs to an instance that has restarted.
func TestUnknownApproval(t *testing.T) {
	t.Parallel()

	box := writeToolBox()
	runner := NewRunner(box, DefaultMaxIterations, DefaultTimeout, DefaultMaxConcurrentChats)

	chunks, err := collect(t, runner, Request{
		Client:        &fakeModel{answers: []*ai.ChatResult{{Text: "..."}}},
		Conversation:  userAsks("создай правило"),
		ConfirmID:     "0123456789abcdef0123456789abcdef",
		Authorization: "Bearer user-token",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	last := chunks[len(chunks)-1]
	if last.Done == nil || last.Done.StopReason != StopReasonError {
		t.Fatalf("last chunk = %+v, want an error", last)
	}
	if !strings.Contains(last.Done.Error, "no longer waiting") {
		t.Errorf("error = %q, want it to say the approval is not held", last.Done.Error)
	}
}

func TestSplitSecret(t *testing.T) {
	t.Parallel()

	t.Run("takes the secret out", func(t *testing.T) {
		t.Parallel()

		stripped, secret := splitSecret(`{"id":"token-1","secret":{"label":"API token","value":"` + testSecret + `","note":"once"}}`)

		if secret == nil || secret.Value != testSecret || secret.Label != "API token" {
			t.Fatalf("secret = %+v", secret)
		}
		if strings.Contains(stripped, testSecret) {
			t.Errorf("the secret stayed in %q", stripped)
		}
		if !strings.Contains(stripped, "token-1") {
			t.Errorf("the rest of the result was lost: %q", stripped)
		}
	})

	t.Run("leaves a result without one alone", func(t *testing.T) {
		t.Parallel()

		const result = `{"id":"rule-1"}`

		stripped, secret := splitSecret(result)
		if secret != nil || stripped != result {
			t.Errorf("stripped = %q, secret = %+v", stripped, secret)
		}
	})
}

func TestPendingCallsExpire(t *testing.T) {
	t.Parallel()

	now := testClock()
	pending := newPendingCalls(confirmationTTL, maxPendingConfirmations)
	pending.now = now.now

	id, err := pending.add(ai.ToolCall{Name: toolCreateRule}, "Create a policy rule", ownerOf("Bearer user-token"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	now.advance(confirmationTTL + 1)

	if _, _, err := pending.take(id, ownerOf("Bearer user-token")); !errors.Is(err, ErrUnknownConfirmation) {
		t.Errorf("err = %v, want the approval to have expired", err)
	}
}

// confirmationID digs the identifier the user would send back out of a run.
func confirmationID(t *testing.T, chunks []Chunk) string {
	t.Helper()

	for _, chunk := range chunks {
		if chunk.Confirmation != nil {
			return chunk.Confirmation.ID
		}
	}

	t.Fatal("the run asked for no confirmation")

	return ""
}
