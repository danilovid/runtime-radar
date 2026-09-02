// Package auth verifies the bearer token an MCP client authenticates with and
// carries that very token over to the gRPC calls the tools make, so that RBAC
// and audit of History API and Event Processor see the human behind the agent
// rather than the MCP server itself.
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"google.golang.org/grpc/metadata"
)

const (
	authorizationHeader = "authorization"
	bearerPrefix        = "Bearer "

	// AnonymousUser is reported as the caller when auth is disabled.
	AnonymousUser = "anonymous"
)

// ErrNoToken is returned when auth is enabled but the client sent no token.
var ErrNoToken = errors.New("no bearer token provided")

// Caller identifies whoever made the tool call. It is what the call log names
// and what the tools writing through Public API act as; it deliberately carries
// no credential, so that nothing here can end up in a log line.
type Caller struct {
	Username string
	UserID   string
}

type contextKey struct{ name string }

var (
	callerContextKey = &contextKey{name: "caller"}
	tokenContextKey  = &contextKey{name: "token"}
)

// WithCaller returns a context naming the caller a tool runs for. Authorize
// does this for every call it lets through; it is exported so that a handler
// can be exercised without an authorizer.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerContextKey, caller)
}

// CallerFromContext returns the caller a tool is running for. It is set by
// Authorize, so a handler reached through addTool always has one.
func CallerFromContext(ctx context.Context) Caller {
	caller, _ := ctx.Value(callerContextKey).(Caller)

	return caller
}

// BearerFromContext returns the caller's verified token. Only the clients
// talking to the product's own services may use it: it is the caller's
// credential, and it is never logged or sent anywhere else.
func BearerFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenContextKey).(string)

	return token
}

// Authorizer checks tokens against the product's JWT key. A zero-value
// Authorizer (auth disabled) lets every call through.
type Authorizer struct {
	verifier jwt.Verifier
	key      []byte
	enabled  bool
	// exchanger resolves MCP keys, which are not JWTs, into the short-lived
	// JWT of the user they were issued to. Nil when Public API is not
	// configured, in which case only JWTs are accepted.
	exchanger *KeyExchanger
}

// WithKeyExchanger lets the authorizer accept MCP keys issued by Public API in
// addition to the product's own JWTs.
func (a *Authorizer) WithKeyExchanger(exchanger *KeyExchanger) *Authorizer {
	a.exchanger = exchanger

	return a
}

// New builds an Authorizer. When enabled is false tokenKey is not needed and
// tokens, if any, are passed through without verification.
func New(enabled bool, tokenKey string) (*Authorizer, error) {
	if !enabled {
		return &Authorizer{}, nil
	}

	verifier, key, err := jwt.NewKeyVerifier(tokenKey)
	if err != nil {
		return nil, fmt.Errorf("can't instantiate key verifier: %w", err)
	}

	return &Authorizer{verifier: verifier, key: key, enabled: true}, nil
}

// Enabled reports whether tokens are verified.
func (a *Authorizer) Enabled() bool {
	return a.enabled
}

// Authorize verifies that token grants every permission in perms and returns a
// context that carries the token to outgoing gRPC calls. Passing no permissions
// only checks that the token itself is valid.
//
// The token is never logged and never leaves the service other than towards the
// product's own gRPC services.
func (a *Authorizer) Authorize(ctx context.Context, token string, perms ...Permission) (context.Context, Caller, error) {
	if !a.enabled {
		caller := Caller{Username: AnonymousUser}
		ctx = WithCaller(withToken(ctx, token), caller)

		return context.WithValue(ctx, tokenContextKey, token), caller, nil
	}

	// Scopes belong to an MCP key. A session opened with the product's own JWT
	// has none and is limited by the role behind that JWT alone.
	var scopes []string

	if token == "" {
		return ctx, Caller{}, ErrNoToken
	}

	// An MCP key is not a JWT and nothing downstream understands it, so it is
	// exchanged for the JWT of the user it belongs to. From here on the two
	// kinds of credential are the same thing.
	if !IsJWT(token) {
		if a.exchanger == nil {
			return ctx, Caller{}, fmt.Errorf("%w: mcp keys are not configured", jwt.ErrUnauthenticated)
		}

		exchanged, keyScopes, err := a.exchanger.Exchange(ctx, token)
		if err != nil {
			return ctx, Caller{}, err
		}

		token = exchanged
		scopes = keyScopes
	}

	// The lib verifier reads the token from incoming gRPC metadata, which is
	// where a token lives in every other service of the product. Reusing it
	// keeps one implementation of "is this token any good" in the codebase.
	mdCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(authorizationHeader, bearerPrefix+token))

	parsed, err := jwt.TokenFromContext(mdCtx, a.key)
	if err != nil {
		return ctx, Caller{}, fmt.Errorf("%w: %w", jwt.ErrUnauthenticated, err)
	}

	for _, p := range perms {
		if err := a.verifier.VerifyPermission(mdCtx, p.Type, p.Actions...); err != nil {
			return ctx, Caller{}, err
		}
	}

	caller := Caller{Username: parsed.Username, UserID: parsed.UserID}

	ctx = WithCaller(withToken(ctx, token), caller)
	ctx = context.WithValue(ctx, tokenContextKey, token)
	ctx = WithScopes(ctx, scopes)

	return ctx, caller, nil
}

// Permission is a permission a tool requires from the caller's token.
type Permission struct {
	Type    jwt.PermissionType
	Actions []jwt.Action
}

// ReadEvents is the permission required to read runtime events and their stats.
func ReadEvents() Permission {
	return Permission{Type: jwt.PermissionEvents, Actions: []jwt.Action{jwt.ActionRead}}
}

// ReadSystemSettings is the permission required to read the detector list.
func ReadSystemSettings() Permission {
	return Permission{Type: jwt.PermissionSystemSettings, Actions: []jwt.Action{jwt.ActionRead}}
}

// WriteSystemSettings is the permission required to change the set of admission
// sources. The product guards both monitors' configuration with system
// settings; which of the two an MCP key may touch is decided by its scope.
func WriteSystemSettings() Permission {
	return Permission{Type: jwt.PermissionSystemSettings, Actions: []jwt.Action{jwt.ActionUpdate}}
}

// ReadRules is the permission required to list policy rules.
func ReadRules() Permission {
	return Permission{Type: jwt.PermissionRules, Actions: []jwt.Action{jwt.ActionRead}}
}

// CreateRules is the permission required to add a policy rule.
func CreateRules() Permission {
	return Permission{Type: jwt.PermissionRules, Actions: []jwt.Action{jwt.ActionCreate}}
}

// UpdateRules is the permission required to change a policy rule.
func UpdateRules() Permission {
	return Permission{Type: jwt.PermissionRules, Actions: []jwt.Action{jwt.ActionUpdate}}
}

// DeleteRules is the permission required to remove a policy rule.
func DeleteRules() Permission {
	return Permission{Type: jwt.PermissionRules, Actions: []jwt.Action{jwt.ActionDelete}}
}

// ReadNotifications is the permission required to list notification templates.
func ReadNotifications() Permission {
	return Permission{Type: jwt.PermissionNotifications, Actions: []jwt.Action{jwt.ActionRead}}
}

// CreateNotifications is the permission required to add a notification template.
func CreateNotifications() Permission {
	return Permission{Type: jwt.PermissionNotifications, Actions: []jwt.Action{jwt.ActionCreate}}
}

// ReadIntegrations is the permission required to list the notification services
// templates are sent through. It never exposes their credentials.
func ReadIntegrations() Permission {
	return Permission{Type: jwt.PermissionIntegrations, Actions: []jwt.Action{jwt.ActionRead}}
}

// CreateIntegrations is the permission required to connect a notification
// service.
func CreateIntegrations() Permission {
	return Permission{Type: jwt.PermissionIntegrations, Actions: []jwt.Action{jwt.ActionCreate}}
}

// ReadAPITokens is the permission required to list the caller's API tokens.
func ReadAPITokens() Permission {
	return Permission{Type: jwt.PermissionPublicAccessTokens, Actions: []jwt.Action{jwt.ActionRead}}
}

// CreateAPITokens is the permission required to issue an API token.
func CreateAPITokens() Permission {
	return Permission{Type: jwt.PermissionPublicAccessTokens, Actions: []jwt.Action{jwt.ActionCreate}}
}

// DeleteAPITokens is the permission required to revoke an API token.
func DeleteAPITokens() Permission {
	return Permission{Type: jwt.PermissionPublicAccessTokens, Actions: []jwt.Action{jwt.ActionDelete}}
}

// withToken attaches the token to outgoing gRPC metadata, mirroring what
// jwt.GeneratePerRPCCredentials does for service-to-service calls.
func withToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, authorizationHeader, bearerPrefix+token)
}

// TokenFromHeader extracts a bearer token from the value of an HTTP
// Authorization header. It returns an empty string when there is none.
func TokenFromHeader(header string) string {
	if len(header) <= len(bearerPrefix) {
		return ""
	}

	// The scheme is case-insensitive per RFC 7235, but every client of this
	// product sends it capitalized; accepting the exact form keeps parsing
	// trivial and matches lib/security/jwt.
	if header[:len(bearerPrefix)] != bearerPrefix {
		return ""
	}

	return header[len(bearerPrefix):]
}
