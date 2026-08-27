package service

import (
	"context"
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/public-api/pkg/model"
	"google.golang.org/grpc/metadata"
)

// fakeExchanger stands in for the exchange itself and records whether it ran.
type fakeExchanger struct {
	called bool
}

func (f *fakeExchanger) ExchangeMCPKey(context.Context, string) (*model.ExchangeMCPKeyResp, error) {
	f.called = true

	return &model.ExchangeMCPKeyResp{AccessToken: "signed"}, nil
}

var (
	exchangeKey  = []byte("0123456789abcdef")
	strangerKey  = []byte("fedcba9876543210")
	exchangeUser = "mcp-server"
)

// callerContext builds the context the HTTP middleware hands to the service:
// the Authorization header, in incoming gRPC metadata.
func callerContext(t *testing.T, key []byte, roleName string) context.Context {
	t.Helper()

	token := &jwt.Token{
		Username:  exchangeUser,
		TokenType: jwt.TokenTypeAccess,
		Role:      &jwt.Role{RoleName: roleName},
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: &jwtv5.NumericDate{Time: time.Now().Add(time.Hour)},
			IssuedAt:  &jwtv5.NumericDate{Time: time.Now()},
		},
	}

	signed, err := jwt.SignHS256(token, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	return bearerContext(signed)
}

// serviceContext carries the credential MCP Server actually presents: the one
// lib/security/jwt issues to a service, with no permissions of its own. The
// contract between the two services is what this pins down.
func serviceContext(t *testing.T, key []byte) context.Context {
	t.Helper()

	signed, err := jwt.GenerateServiceToken(key, exchangeUser, &jwt.RolePermissions{})
	if err != nil {
		t.Fatalf("generate service token: %v", err)
	}

	return bearerContext(signed)
}

func bearerContext(signed string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+signed))
}

func TestExchangeMCPKeyRequiresAServiceCaller(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		wantErr bool
	}{
		{
			name:    "no token at all",
			ctx:     func(*testing.T) context.Context { return context.Background() },
			wantErr: true,
		},
		{
			name:    "token signed with another key",
			ctx:     func(t *testing.T) context.Context { return callerContext(t, strangerKey, serviceRoleName) },
			wantErr: true,
		},
		{
			// A stolen key must not be exchangeable by whoever holds a session
			// of their own: the exchange is between services.
			name:    "a user's own session",
			ctx:     func(t *testing.T) context.Context { return callerContext(t, exchangeKey, "administrator") },
			wantErr: true,
		},
		{
			name: "a service of the product",
			ctx:  func(t *testing.T) context.Context { return callerContext(t, exchangeKey, serviceRoleName) },
		},
		{
			name: "the token MCP Server presents",
			ctx:  func(t *testing.T) context.Context { return serviceContext(t, exchangeKey) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			next := &fakeExchanger{}
			svc := &MCPKeyExchangeAuth{MCPKeyExchanger: next, TokenKey: exchangeKey}

			_, err := svc.ExchangeMCPKey(test.ctx(t), "some-mcp-key")

			switch {
			case test.wantErr && err == nil:
				t.Fatal("the exchange was allowed")
			case test.wantErr && !errors.Is(err, jwt.ErrUnauthenticated):
				t.Errorf("error = %v, want an unauthenticated one", err)
			case !test.wantErr && err != nil:
				t.Fatalf("the exchange was refused: %v", err)
			}

			if next.called != !test.wantErr {
				t.Errorf("the key reached the exchange = %v, want %v", next.called, !test.wantErr)
			}
		})
	}
}
