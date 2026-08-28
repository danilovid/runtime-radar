package ai

import (
	"encoding/json"
	"regexp"
)

const (
	redactedMarker = "[REDACTED]"

	// Words that name a credential in the command lines and environment dumps
	// runtime events are made of.
	secretWordPattern = `(?:password|passwd|pwd|token|api[_-]?key|apikey|access[_-]?key|secret[_-]?key|secret)`

	// What such a word may be followed by: a quoted string, or a bare argument
	// up to the next separator.
	secretValuePattern = `("[^"]*"|'[^']*'|[^\s,;&"']+)`
)

var (
	// A PEM block is matched first: it's the only pattern whose body would
	// otherwise be eaten piecemeal by the base64 rule below, leaving the
	// give-away header in place.
	pemRE = regexp.MustCompile(`(?s)-----BEGIN [^-\n]*-----.*?-----END [^-\n]*-----`)

	// "Authorization: Bearer x", "authorization=Basic x" and the bare scheme
	// prefixes that show up in process arguments (curl -H "Bearer x").
	authorizationRE = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)?\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)

	// key=value and key: value forms, quoted or bare. The key is kept so the
	// model still sees that a credential was passed, just not which one.
	assignmentRE = regexp.MustCompile(`(?i)(\b` + secretWordPattern + `\b\s*[:=]\s*)` + secretValuePattern)

	// The same words as a command-line flag whose value is the next argument:
	// "--password hunter2". A leading dash is required, so that the word
	// occurring in ordinary text doesn't swallow whatever follows it.
	secretFlagRE = regexp.MustCompile(`(?i)(--?[a-z0-9-]*` + secretWordPattern + `[a-z0-9-]*\s+)` + secretValuePattern)

	// Long unbroken base64: keys, certificates and dumps pasted into arguments.
	// The bound is high enough that ordinary identifiers and hashes stay visible.
	longBase64RE = regexp.MustCompile(`[A-Za-z0-9+/]{64,}={0,2}`)

	// JSON keys whose value is a secret whatever it looks like.
	secretKeyRE = regexp.MustCompile(`(?i)^(.*_)?(password|passwd|pwd|token|api[_-]?key|apikey|secret|credentials?|authorization)(_.*)?$`)
)

// RedactEventJSON masks credential-shaped text inside a runtime event before it
// is handed to a model provider. Runtime telemetry carries whole command lines
// and environment dumps, so an event may quote a token that must not leave the
// deployment even though the event itself is worth analysing.
//
// Values are redacted, keys are not: the model still needs the shape of the
// event to explain it. A document that isn't valid JSON (a truncated dump, or
// the deprecated client-supplied payload) is redacted as plain text rather than
// passed through, because it can carry exactly the same secrets.
func RedactEventJSON(eventJSON string) string {
	var doc any
	if err := json.Unmarshal([]byte(eventJSON), &doc); err != nil {
		return redactText(eventJSON)
	}

	redacted, err := json.Marshal(redactValue(doc, false))
	if err != nil {
		return redactText(eventJSON)
	}

	return string(redacted)
}

// redactValue walks a decoded JSON document. underSecretKey reports whether the
// value being visited was reached through a key that names a credential, in
// which case the whole value goes rather than the patterns inside it.
func redactValue(value any, underSecretKey bool) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = redactValue(item, underSecretKey || secretKeyRE.MatchString(key))
		}

		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, redactValue(item, underSecretKey))
		}

		return result
	case string:
		if underSecretKey && typed != "" {
			return redactedMarker
		}

		return redactText(typed)
	default:
		// Numbers, booleans and nulls can't carry a credential in any form
		// this function is able to recognise.
		return value
	}
}

func redactText(text string) string {
	text = pemRE.ReplaceAllString(text, redactedMarker)
	text = authorizationRE.ReplaceAllString(text, "${1}${2} "+redactedMarker)
	text = assignmentRE.ReplaceAllString(text, "${1}"+redactedMarker)
	text = secretFlagRE.ReplaceAllString(text, "${1}"+redactedMarker)
	text = longBase64RE.ReplaceAllString(text, redactedMarker)

	return text
}
