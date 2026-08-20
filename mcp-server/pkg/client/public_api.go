package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
)

const (
	// publicAPITimeout bounds one call to Public API.
	publicAPITimeout = 15 * time.Second

	// accessTokenPath is where Public API keeps the product's API tokens.
	accessTokenPath = "/api/v1/access-token"

	// tokenPageSize is how many tokens one listing returns. A user with more
	// than this many API tokens has a problem the assistant cannot fix.
	tokenPageSize = 100

	// maxErrorBodyBytes bounds what is read from a failed response before it is
	// turned into an error message.
	maxErrorBodyBytes = 4 * 1024

	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// PublicAPI talks to Public API's REST interface on behalf of the caller. The
// caller's own token authenticates every request, so RBAC and audit there name
// the person behind the agent, exactly as they do for the gRPC clients.
type PublicAPI struct {
	baseURL    string
	httpClient *http.Client
}

// NewPublicAPI builds a client for the Public API at baseURL.
func NewPublicAPI(baseURL string, tlsConfig *tls.Config) *PublicAPI {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	// Public API is an internal service here; an outbound proxy has no
	// business intercepting this.
	transport.Proxy = nil

	return &PublicAPI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: publicAPITimeout, Transport: transport},
	}
}

// APIToken is one of the product's API tokens, without its secret: the secret
// exists only in the answer that created it.
type APIToken struct {
	ID            string               `json:"id"`
	Kind          string               `json:"kind"`
	Name          string               `json:"name"`
	UserID        string               `json:"user_id"`
	Permissions   *jwt.RolePermissions `json:"permissions"`
	ExpiresAt     *time.Time           `json:"expires_at,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	InvalidatedAt *time.Time           `json:"invalidated_at,omitempty"`
}

// CreateAPITokenReq asks Public API for a new token.
type CreateAPITokenReq struct {
	Name        string               `json:"name"`
	UserID      string               `json:"user_id"`
	ExpiresAt   *time.Time           `json:"expires_at"`
	Permissions *jwt.RolePermissions `json:"permissions"`
}

// CreateAPITokenResp is what Public API answers: the identifier of the token
// and its secret, which is shown once and never stored in readable form.
type CreateAPITokenResp struct {
	ID          string `json:"id"`
	AccessToken string `json:"access_token"`
}

type listAPITokensResp struct {
	Total        int         `json:"total"`
	AccessTokens []*APIToken `json:"access_tokens"`
}

// ListAPITokens returns the caller's own API tokens.
func (p *PublicAPI) ListAPITokens(ctx context.Context, bearer string) ([]*APIToken, int, error) {
	path := fmt.Sprintf("%s/page/1?page_size=%d", accessTokenPath, tokenPageSize)

	var resp listAPITokensResp
	if err := p.do(ctx, http.MethodGet, path, bearer, nil, &resp); err != nil {
		return nil, 0, err
	}

	return resp.AccessTokens, resp.Total, nil
}

// CreateAPIToken issues a token for the caller.
func (p *PublicAPI) CreateAPIToken(ctx context.Context, bearer string, req *CreateAPITokenReq) (*CreateAPITokenResp, error) {
	var resp CreateAPITokenResp
	if err := p.do(ctx, http.MethodPost, accessTokenPath, bearer, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// DeleteAPIToken revokes one of the caller's tokens.
func (p *PublicAPI) DeleteAPIToken(ctx context.Context, bearer, id string) error {
	return p.do(ctx, http.MethodDelete, accessTokenPath+"/"+id, bearer, nil, nil)
}

func (p *PublicAPI) do(ctx context.Context, method, path, bearer string, body, out any) error {
	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("can't encode the request: %w", err)
		}

		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("can't build the request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if bearer != "" {
		req.Header.Set(authorizationHeader, bearerPrefix+bearer)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach public api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		// The body is Public API's own error envelope; it names what was
		// refused without echoing the credential.
		message, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))

		return fmt.Errorf("public api answered %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("can't decode the answer of public api: %w", err)
	}

	return nil
}
