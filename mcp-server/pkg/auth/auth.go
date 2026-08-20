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

// Caller identifies whoever made the tool call, for logging purposes only.
type Caller struct {
	Username string
	UserID   string
}

// Authorizer checks tokens against the product's JWT key. A zero-value
// Authorizer (auth disabled) lets every call through.
type Authorizer struct {
	verifier jwt.Verifier
	key      []byte
	enabled  bool
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
		return withToken(ctx, token), Caller{Username: AnonymousUser}, nil
	}

	if token == "" {
		return ctx, Caller{}, ErrNoToken
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

	return withToken(ctx, token), caller, nil
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
