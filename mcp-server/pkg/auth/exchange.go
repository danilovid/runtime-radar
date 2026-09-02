package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/build"
)

const (
	// exchangePath is an internal route of Public API: it is not routed by the
	// reverse proxy, so it is reachable inside the deployment only.
	exchangePath = "/internal/v1/mcp-key/exchange"

	exchangeTimeout = 10 * time.Second

	// exchangeRenewBefore re-exchanges a key slightly before the JWT it was
	// exchanged for expires, so a long-running call never carries a token that
	// dies mid-flight.
	exchangeRenewBefore = time.Minute
)

// KeyExchanger turns an MCP key into a short-lived JWT of the user it belongs
// to. Everything downstream of this service speaks JWT, so a key is exchanged
// once and the result is reused until it is nearly expired.
type KeyExchanger struct {
	endpoint   string
	httpClient *http.Client
	// serviceToken authenticates this service to Public API. The endpoint is
	// internal, and this keeps it from answering anyone who can reach the pod.
	serviceToken string

	mu     sync.Mutex
	cached map[string]exchanged
}

type exchanged struct {
	token string
	// scopes are the key's own, they are not part of the JWT: the key is a
	// credential of this server, and what it may reach is decided here.
	scopes    []string
	expiresAt time.Time
}

// NewKeyExchanger builds an exchanger against Public API. An empty tokenKey
// means the deployment runs without auth, in which case no service token is
// sent.
func NewKeyExchanger(publicAPIURL string, tlsConfig *tls.Config, tokenKey []byte) (*KeyExchanger, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.Proxy = nil

	exchanger := &KeyExchanger{
		endpoint:   strings.TrimRight(publicAPIURL, "/") + exchangePath,
		httpClient: &http.Client{Timeout: exchangeTimeout, Transport: transport},
		cached:     make(map[string]exchanged),
	}

	if len(tokenKey) > 0 {
		// The service token proves the caller is part of the product; the key
		// being exchanged is what actually decides the permissions.
		token, err := jwt.GenerateServiceToken(tokenKey, build.AppName, &jwt.RolePermissions{})
		if err != nil {
			return nil, fmt.Errorf("can't generate service token: %w", err)
		}

		exchanger.serviceToken = token
	}

	return exchanger, nil
}

// Exchange returns a JWT for the given key along with the scopes the key was
// issued with, reusing a previous exchange while it is still valid.
func (e *KeyExchanger) Exchange(ctx context.Context, key string) (string, []string, error) {
	if entry, ok := e.fromCache(key); ok {
		return entry.token, entry.scopes, nil
	}

	payload, err := json.Marshal(map[string]string{"key": key})
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if e.serviceToken != "" {
		req.Header.Set(authorizationHeader, bearerPrefix+e.serviceToken)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("can't reach public api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The body may name the reason; it is not passed on, so that a caller
		// can't tell an unknown key from a revoked one.
		return "", nil, fmt.Errorf("%w: mcp key was not accepted", jwt.ErrUnauthenticated)
	}

	var answer struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
		Scopes      []string  `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return "", nil, fmt.Errorf("can't decode exchange response: %w", err)
	}

	if answer.AccessToken == "" {
		return "", nil, fmt.Errorf("%w: empty token from public api", jwt.ErrUnauthenticated)
	}

	e.store(key, answer.AccessToken, answer.Scopes, answer.ExpiresAt)

	return answer.AccessToken, answer.Scopes, nil
}

func (e *KeyExchanger) fromCache(key string) (exchanged, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry, ok := e.cached[key]
	if !ok || time.Now().Add(exchangeRenewBefore).After(entry.expiresAt) {
		return exchanged{}, false
	}

	return entry, true
}

func (e *KeyExchanger) store(key, token string, scopes []string, expiresAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.cached[key] = exchanged{token: token, scopes: scopes, expiresAt: expiresAt}
}

// IsJWT reports whether a bearer value looks like one of the product's tokens
// rather than an MCP key. Keys are hex, JWTs have three dot-separated parts.
func IsJWT(token string) bool {
	return strings.Count(token, ".") == 2
}
