package service

import (
	"context"

	"github.com/runtime-radar/runtime-radar/public-api/pkg/model"
)

// MCPKeyExchanger turns an MCP key into a short-lived JWT for its owner. It is
// implemented by the token verifier, which already knows how keys are hashed
// and how a key's permissions meet the user's role.
type MCPKeyExchanger interface {
	ExchangeMCPKey(ctx context.Context, key string) (*model.ExchangeMCPKeyResp, error)
}
