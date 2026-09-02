package assistant

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
)

const (
	// maxSuggestions is how many follow-ups are offered. Three fit under an
	// answer without turning into a menu.
	maxSuggestions = 3
	// maxSuggestionRunes keeps a follow-up to something readable on a button.
	maxSuggestionRunes = 60
)

// suggestPrompt asks for the follow-ups. It is a separate, short request rather
// than a trailer on the answer: a trailer leaks into what the user reads when
// the model formats it differently than asked, and it would have to be cut back
// out of the stream that is already on screen.
const suggestPrompt = `Read the exchange above. Propose the questions the user is most likely to ask next.

Rules:
- At most 3, and fewer when fewer make sense. None at all is a valid answer.
- Each is a question the user would ask, written in Russian, at most 60 characters.
- They follow from this answer. Do not repeat what was already answered, and do not invent facts.
- Answer with a JSON array of strings and nothing else: ["...", "..."]`

// suggest asks the model what the user is likely to ask next. It never fails a
// turn: the answer is already delivered, and a chat without follow-ups is only
// missing a convenience.
func (r *Runner) suggest(ctx context.Context, req Request, answer string) []string {
	if strings.TrimSpace(answer) == "" {
		return nil
	}

	messages := append(conversationForSuggest(req.Conversation, answer),
		ai.Message{Role: ai.RoleUser, Content: suggestPrompt})

	var raw strings.Builder

	// No tools are offered: this call decides nothing and must not read
	// anything, and a model that cannot call tools cannot be talked into it.
	_, err := req.Client.Chat(ctx, messages, nil, func(delta string) {
		raw.WriteString(delta)
	})
	if err != nil {
		log.Debug().Err(err).Msg("Can't get follow-up suggestions")

		return nil
	}

	return parseSuggestions(raw.String())
}

// conversationForSuggest is the exchange the follow-ups are about: the question
// and the answer to it, without the history before them. What came earlier does
// not change what to ask next, and sending it all would pay for the whole
// conversation twice on every turn.
func conversationForSuggest(conversation []ai.Message, answer string) []ai.Message {
	messages := make([]ai.Message, 0, 2)

	for i := len(conversation) - 1; i >= 0; i-- {
		if conversation[i].Role == ai.RoleUser {
			messages = append(messages, conversation[i])

			break
		}
	}

	return append(messages, ai.Message{Role: ai.RoleAssistant, Content: answer})
}

// parseSuggestions reads the array the model was asked for. Anything else is
// dropped rather than shown: a button is not the place to find out that a model
// answered with prose.
func parseSuggestions(raw string) []string {
	text := strings.TrimSpace(raw)

	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")

	if start < 0 || end <= start {
		return nil
	}

	var parsed []string
	if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err != nil {
		log.Debug().Err(err).Msg("Can't parse follow-up suggestions")

		return nil
	}

	suggestions := make([]string, 0, maxSuggestions)

	for _, suggestion := range parsed {
		suggestion = strings.TrimSpace(suggestion)
		if suggestion == "" || len([]rune(suggestion)) > maxSuggestionRunes {
			continue
		}

		suggestions = append(suggestions, suggestion)

		if len(suggestions) == maxSuggestions {
			break
		}
	}

	return suggestions
}
