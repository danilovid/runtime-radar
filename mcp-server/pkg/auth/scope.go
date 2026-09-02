package auth

import "context"

// Scope narrows what an MCP key may reach. Unlike a permission, which the
// product's roles define and every service enforces, a scope belongs to the key
// alone and is checked here: the key is a separate credential from a public API
// token, so it can carry a vocabulary of its own without touching the roles.
//
// A key with no scopes is unrestricted, which is what every key issued before
// scopes existed is. A session opened with the product's own JWT — a human
// signed into the UI — carries no key and is unrestricted as well.
type Scope string

const (
	// ScopeRuntimeMonitor covers the runtime side: events Tetragon recorded,
	// the detectors that flag them and their statistics.
	ScopeRuntimeMonitor Scope = "runtime_monitor"
	// ScopeAdmission covers the admission side: the Kyverno sources
	// admission-monitor keeps applied and the findings they produced.
	ScopeAdmission Scope = "admission"
)

// ScopeMetaKey is the _meta field a tool publishes its scope under, so that a
// client can group the tools it offers without hardcoding their names.
const ScopeMetaKey = "runtime-radar/scope"

// Scopes are every scope this server knows, in the spelling a key declares.
var Scopes = []Scope{ScopeRuntimeMonitor, ScopeAdmission}

var scopesContextKey = &contextKey{name: "scopes"}

// IsScope reports whether name is a scope this server knows.
func IsScope(name string) bool {
	for _, scope := range Scopes {
		if string(scope) == name {
			return true
		}
	}

	return false
}

// WithScopes returns a context carrying the scopes of the key that opened the
// session. An empty slice means the session is unrestricted.
func WithScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, scopesContextKey, scopes)
}

// AllowsScope reports whether the session may use a tool belonging to scope.
// A session that declared no scopes may use every tool.
func AllowsScope(ctx context.Context, scope Scope) bool {
	scopes, _ := ctx.Value(scopesContextKey).([]string)
	if len(scopes) == 0 {
		return true
	}

	for _, granted := range scopes {
		if granted == string(scope) {
			return true
		}
	}

	return false
}
