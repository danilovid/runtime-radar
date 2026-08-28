package model

import (
	"time"

	"github.com/google/uuid"
)

type CreateAccessTokenReq struct {
	Name        string       `json:"name"`
	UserID      uuid.UUID    `json:"user_id"`
	ExpiresAt   *time.Time   `json:"expires_at"`
	Permissions *Permissions `json:"permissions"`
	// Scopes narrow an MCP key to one half of the product. They are ignored for
	// a public API token, which has no such halves.
	Scopes Scopes `json:"scopes,omitempty"`
	// Kind is set by the handler, not by the client: it follows the route the
	// request came in on.
	Kind TokenKind `json:"-"`
}

type CreateAccessTokenResp struct {
	ID          uuid.UUID `json:"id"`
	AccessToken string    `json:"access_token"`
}

// ExchangeMCPKeyReq is an internal request from MCP Server: it hands over the
// key its caller presented and gets a short-lived JWT for that user back.
type ExchangeMCPKeyReq struct {
	Key string `json:"key"`
}

type ExchangeMCPKeyResp struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Username    string    `json:"username"`
	UserID      uuid.UUID `json:"user_id"`
	// Scopes are the key's own and are not part of the token: MCP Server is
	// what enforces them, so they travel next to it.
	Scopes Scopes `json:"scopes,omitempty"`
}

type ListAccessTokenResp struct {
	Total        int                `json:"total"`
	AccessTokens []*AccessTokenResp `json:"access_tokens"`
}

type AccessTokenResp struct {
	ID            uuid.UUID    `json:"id"`
	Kind          TokenKind    `json:"kind"`
	Name          string       `json:"name"`
	UserID        uuid.UUID    `json:"user_id"`
	Permissions   *Permissions `json:"permissions"`
	Scopes        Scopes       `json:"scopes,omitempty"`
	ExpiresAt     *time.Time   `json:"expires_at,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	InvalidatedAt *time.Time   `json:"invalidated_at,omitempty"`
}
