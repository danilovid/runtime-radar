package assistant

import (
	"reflect"
	"strings"
	"testing"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
)

func TestParseSuggestions(t *testing.T) {
	long := strings.Repeat("я", maxSuggestionRunes+1)

	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "plain array", raw: `["Что дальше?", "Кто это сделал?"]`, want: []string{"Что дальше?", "Кто это сделал?"}},
		{
			name: "wrapped in prose and a fence",
			raw:  "Вот вопросы:\n```json\n[\"Что дальше?\"]\n```",
			want: []string{"Что дальше?"},
		},
		{name: "no array at all", raw: "Мне нечего предложить.", want: nil},
		{name: "not an array of strings", raw: `[{"q":"a"}]`, want: nil},
		{name: "empty entries are dropped", raw: `["", "  ", "Что дальше?"]`, want: []string{"Что дальше?"}},
		{name: "an overlong entry is dropped", raw: `["` + long + `", "Что дальше?"]`, want: []string{"Что дальше?"}},
		{
			name: "more than three are cut",
			raw:  `["1", "2", "3", "4"]`,
			want: []string{"1", "2", "3"},
		},
	}

	for _, tc := range cases {
		got := parseSuggestions(tc.raw)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}

		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: parseSuggestions() = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

// The follow-up request is about the exchange that just happened, not about the
// whole conversation: everything before it is paid for on every turn otherwise.
func TestConversationForSuggest(t *testing.T) {
	conversation := []ai.Message{
		{Role: ai.RoleUser, Content: "первый вопрос"},
		{Role: ai.RoleAssistant, Content: "первый ответ"},
		{Role: ai.RoleUser, Content: "второй вопрос"},
	}

	messages := conversationForSuggest(conversation, "второй ответ")

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want the last question and the answer", len(messages))
	}

	if messages[0].Content != "второй вопрос" {
		t.Errorf("question = %q, want the last one", messages[0].Content)
	}

	if messages[1].Role != ai.RoleAssistant || messages[1].Content != "второй ответ" {
		t.Errorf("answer = %+v", messages[1])
	}
}
