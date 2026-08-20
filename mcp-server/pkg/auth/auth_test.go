package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"google.golang.org/grpc/metadata"
)

// tokenKey is a 32-byte hex key, the size the product uses for JWT signing.
const tokenKey = "dd20ed6d406da2a2a325dfe6e74a3f941e94c154b0b8d15b7360e1cce46a522a"

func serviceToken(t *testing.T, permissions *jwt.RolePermissions) string {
	t.Helper()

	key, err := cipher.ParseKey(tokenKey)
	if err != nil {
		t.Fatalf("can't parse the key: %v", err)
	}

	token, err := jwt.GenerateServiceToken(key, "tester", permissions)
	if err != nil {
		t.Fatalf("can't generate a token: %v", err)
	}

	return token
}

func TestTokenFromHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{name: "bearer token", header: "Bearer abc.def.ghi", want: "abc.def.ghi"},
		{name: "no header", header: "", want: ""},
		{name: "no scheme", header: "abc.def.ghi", want: ""},
		{name: "another scheme", header: "Basic dXNlcjpwYXNz", want: ""},
		{name: "scheme without a token", header: "Bearer ", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := TokenFromHeader(tc.header); got != tc.want {
				t.Errorf("token = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthorizeDisabled(t *testing.T) {
	t.Parallel()

	authorizer, err := New(false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authorizer.Enabled() {
		t.Error("auth reports itself enabled")
	}

	ctx, caller, err := authorizer.Authorize(context.Background(), "", ReadEvents())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller.Username != AnonymousUser {
		t.Errorf("caller = %+v, want the anonymous caller", caller)
	}
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Error("a call without a token must not carry authorization metadata")
	}
}

func TestAuthorizePropagatesTheToken(t *testing.T) {
	t.Parallel()

	authorizer, err := New(true, tokenKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token := serviceToken(t, &jwt.RolePermissions{
		Events: &jwt.Permission{Actions: []jwt.Action{jwt.ActionRead}},
	})

	ctx, caller, err := authorizer.Authorize(context.Background(), token, ReadEvents())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if caller.Username != "tester" {
		t.Errorf("caller = %+v, want the username from the token", caller)
	}

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("the token was not attached to outgoing calls")
	}

	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer "+token {
		t.Errorf("authorization metadata = %v, want the caller's own token", got)
	}
}

func TestAuthorizeRejects(t *testing.T) {
	t.Parallel()

	authorizer, err := New(true, tokenKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eventsOnly := serviceToken(t, &jwt.RolePermissions{
		Events: &jwt.Permission{Actions: []jwt.Action{jwt.ActionRead}},
	})

	cases := []struct {
		name    string
		token   string
		perms   []Permission
		wantErr error
	}{
		{name: "no token", token: "", perms: []Permission{ReadEvents()}, wantErr: ErrNoToken},
		{name: "garbage token", token: "not-a-jwt", perms: []Permission{ReadEvents()}, wantErr: jwt.ErrUnauthenticated},
		{
			name:    "a permission the token does not grant",
			token:   eventsOnly,
			perms:   []Permission{ReadSystemSettings()},
			wantErr: jwt.ErrPermissionDenied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, _, err := authorizer.Authorize(context.Background(), tc.token, tc.perms...); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestAuthorizeWithoutPermissionsOnlyChecksTheToken(t *testing.T) {
	t.Parallel()

	authorizer, err := New(true, tokenKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// search_docs gates on nothing but a valid token: a role with no product
	// permission at all must still be able to read the documentation.
	token := serviceToken(t, &jwt.RolePermissions{})

	if _, _, err := authorizer.Authorize(context.Background(), token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsABadKey(t *testing.T) {
	t.Parallel()

	if _, err := New(true, "not-hex"); err == nil {
		t.Fatal("error = nil, want a key parsing error")
	}
}
