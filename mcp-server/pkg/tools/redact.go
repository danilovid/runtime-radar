package tools

import "regexp"

// Runtime telemetry carries whole command lines and environment dumps, so an
// event may quote a credential that must not leave the deployment even though
// the event itself is worth analysing by a model. Everything this package hands
// to an MCP client goes through redactSecrets first.
//
// The patterns mirror notifier/pkg/ai/redact.go, which does the same job for
// the built-in "Explain event" flow.
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
)

// redactSecrets masks credential-shaped text. Only values are masked, never the
// words around them: the caller still needs to see that a token was passed.
func redactSecrets(text string) string {
	if text == "" {
		return text
	}

	text = pemRE.ReplaceAllString(text, redactedMarker)
	text = authorizationRE.ReplaceAllString(text, "${1}${2} "+redactedMarker)
	text = assignmentRE.ReplaceAllString(text, "${1}"+redactedMarker)
	text = secretFlagRE.ReplaceAllString(text, "${1}"+redactedMarker)
	text = longBase64RE.ReplaceAllString(text, redactedMarker)

	return text
}
