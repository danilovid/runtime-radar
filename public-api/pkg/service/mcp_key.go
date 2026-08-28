package service

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/public-api/pkg/model"
)

// serviceRoleName is the role lib/security/jwt puts into a service token. Only
// the product's own services may exchange an MCP key: the key decides what the
// resulting session may do, but not who is allowed to ask for it.
const serviceRoleName = "service"

// MCPKeyExchanger turns an MCP key into a short-lived JWT for its owner. It is
// implemented by the token verifier, which already knows how keys are hashed
// and how a key's permissions meet the user's role.
type MCPKeyExchanger interface {
	ExchangeMCPKey(ctx context.Context, key string) (*model.ExchangeMCPKeyResp, error)
}

// MCPKeyExchangeAuth is a layer for jwt-based authentication of the caller. The
// exchange route is internal and the reverse proxy does not expose it, but that
// is a property of one deployment's routing rather than a check: anything able
// to reach this service could otherwise turn a key it stole into a session of
// the user it belongs to.
//
// The credential checked here is the caller's, not the key's. It proves the
// request comes from a service of the product — MCP Server — and nothing more;
// what the resulting session may do still follows from the key and the role of
// its owner.
type MCPKeyExchangeAuth struct {
	// MCPKeyExchanger is the next layer, doing the exchange itself.
	MCPKeyExchanger

	// TokenKey verifies the caller's service token. It is the product's own
	// JWT key, the same one the exchanged token is signed with.
	TokenKey []byte
}

func (a *MCPKeyExchangeAuth) ExchangeMCPKey(ctx context.Context, key string) (*model.ExchangeMCPKeyResp, error) {
	token, err := jwt.TokenFromContext(ctx, a.TokenKey)
	if err != nil {
		log.Warn().Err(err).Msg("Refusing an MCP key exchange: the caller presented no valid token")

		return nil, fmt.Errorf("%w: %w", jwt.ErrUnauthenticated, err)
	}

	if token.Role == nil || token.Role.RoleName != serviceRoleName {
		log.Warn().Str("username", token.Username).
			Msg("Refusing an MCP key exchange: the caller is not a service of the product")

		return nil, fmt.Errorf("%w: only a service of the product may exchange an mcp key", jwt.ErrUnauthenticated)
	}

	return a.MCPKeyExchanger.ExchangeMCPKey(ctx, key)
}
