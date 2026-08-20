package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/lib/security"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/public-api/pkg/model"
)

// mcpKeyTokenTTL bounds the JWT an MCP key is exchanged for. It is short on
// purpose: revoking a key has to take effect quickly, and MCP Server re-exchanges
// as needed.
const mcpKeyTokenTTL = 15 * time.Minute

// ErrNotAnMCPKey is returned when the key exists but belongs to the public API
// rather than to MCP Server. The two are never interchangeable.
var ErrNotAnMCPKey = errors.New("token is not an mcp key")

// MCPKeyExchanger turns an MCP key into a short-lived JWT for the user who owns
// it, carrying the permissions that key and that user have in common.
//
// MCP Server can't use the key itself downstream: History API and Event
// Processor only understand the product's JWTs. Exchanging here keeps one place
// that knows about keys, and keeps the audit trail pointing at the person the
// key was issued to.
type MCPKeyExchanger struct {
	Verifier *Verifier
	TokenKey []byte
}

func (e *MCPKeyExchanger) ExchangeMCPKey(ctx context.Context, key string) (*model.ExchangeMCPKeyResp, error) {
	v := e.Verifier
	tokenKey := e.TokenKey

	if key == "" {
		return nil, fmt.Errorf("%w: no key provided", jwt.ErrUnauthenticated)
	}

	hashed := security.HashSaltedSHA512AsHex([]byte(key), v.AccessTokenSalt)

	at, err := v.AccessTokenRepository.GetByTokenHash(ctx, hashed)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", jwt.ErrUnauthenticated, err)
	}

	if at.Kind != model.TokenKindMCP {
		return nil, ErrNotAnMCPKey
	}

	if at.ExpiresAt != nil && at.ExpiresAt.Before(time.Now()) {
		return nil, jwt.ErrTokenExpired
	}

	if at.InvalidatedAt != nil {
		return nil, ErrTokenInvalidated
	}

	user, err := v.UsersGetter.GetUser(ctx, at.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", jwt.ErrUnauthenticated, err)
	}

	perms := maxCommonPermissions(at.Permissions, user.Role.RolePermissions)
	expiresAt := time.Now().Add(mcpKeyTokenTTL)

	token := &jwt.Token{
		Username:  user.Username,
		UserID:    user.ID,
		TokenType: jwt.TokenTypeAccess,
		Role: &jwt.Role{
			RoleName:        user.Role.RoleName,
			RolePermissions: perms,
		},
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: &jwtv5.NumericDate{Time: expiresAt},
			IssuedAt:  &jwtv5.NumericDate{Time: time.Now()},
		},
	}

	signed, err := jwt.SignHS256(token, tokenKey)
	if err != nil {
		return nil, fmt.Errorf("can't sign token: %w", err)
	}

	userID, err := uuid.Parse(user.ID)
	if err != nil {
		userID = at.UserID
	}

	return &model.ExchangeMCPKeyResp{
		AccessToken: signed,
		ExpiresAt:   expiresAt,
		Username:    user.Username,
		UserID:      userID,
	}, nil
}
