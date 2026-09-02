package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/client"
)

func TestCreateAPIToken(t *testing.T) {
	t.Parallel()

	var received map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer caller-token" {
			t.Errorf("public api got authorization %q, want the caller's own token", r.Header.Get("Authorization"))
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("can't decode the request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"token-1","access_token":"s3cr3t"}`))
	}))
	defer server.Close()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.PublicAPI = client.NewPublicAPI(server.URL, nil)

	ctx := callerContext(t, "user-1", "caller-token")

	result, err := createAPIToken(deps)(ctx, CreateAPITokenArgs{
		Name:        "ci-bot",
		Permissions: []APITokenPermission{{Resource: "Events", Actions: []string{"read"}}},
	})
	if err != nil {
		t.Fatalf("can't create the token: %v", err)
	}

	if result.Secret == nil || result.Secret.Value != "s3cr3t" {
		t.Fatalf("the secret was not returned: %+v", result.Secret)
	}
	if result.Secret.Note == "" {
		t.Error("the secret comes without instructions on how to handle it")
	}
	if len(result.Permissions) != 1 || result.Permissions[0] != permissionEventsRead {
		t.Errorf("granted %v, want [events:read]", result.Permissions)
	}

	// The token belongs to whoever authenticated the session, never to a user
	// the model named.
	if received["user_id"] != "user-1" {
		t.Errorf("token was issued to %v, want user-1", received["user_id"])
	}

	permissions, ok := received["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions are %T, want an object", received["permissions"])
	}
	if _, ok := permissions["events"]; !ok {
		t.Errorf("permissions are %v, want events among them", permissions)
	}
	if _, ok := permissions["rules"]; ok {
		t.Errorf("permissions are %v, want nothing but events", permissions)
	}
	if received["expires_at"] == nil {
		t.Error("the token was issued without an expiry")
	}
}

func TestCreateAPITokenRejectsIncompleteArguments(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.PublicAPI = client.NewPublicAPI("http://public-api:9000", nil)

	readEvents := []APITokenPermission{{Resource: "events", Actions: []string{"read"}}}

	cases := map[string]CreateAPITokenArgs{
		"no name":           {Permissions: readEvents},
		"no permissions":    {Name: "ci-bot"},
		"unknown resource":  {Name: "ci-bot", Permissions: []APITokenPermission{{Resource: "everything", Actions: []string{"read"}}}},
		"unknown action":    {Name: "ci-bot", Permissions: []APITokenPermission{{Resource: "events", Actions: []string{"purge"}}}},
		"lifetime too long": {Name: "ci-bot", Permissions: readEvents, ExpiresInDays: maxTokenLifetimeDays + 1},
		"negative lifetime": {Name: "ci-bot", Permissions: readEvents, ExpiresInDays: -1},
	}

	ctx := callerContext(t, "user-1", "caller-token")

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := createAPIToken(deps)(ctx, args); err == nil {
				t.Error("the arguments were accepted")
			}
		})
	}
}

// TestCreateAPITokenNeedsAUser guards the case where the session authenticated
// as a service rather than a person: there is nobody to issue the token to, and
// guessing would hand out a credential in someone else's name.
func TestCreateAPITokenNeedsAUser(t *testing.T) {
	t.Parallel()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.PublicAPI = client.NewPublicAPI("http://public-api:9000", nil)

	_, err := createAPIToken(deps)(context.Background(), CreateAPITokenArgs{
		Name:        "ci-bot",
		Permissions: []APITokenPermission{{Resource: "events", Actions: []string{"read"}}},
	})
	if err == nil {
		t.Error("a token was issued without a user to issue it to")
	}
}

func TestListAPITokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":2,"access_tokens":[
			{"id":"token-1","kind":"access","name":"ci-bot","created_at":"2026-08-01T00:00:00Z",
			 "permissions":{"events":{"actions":["read"]}}},
			{"id":"key-1","kind":"mcp","name":"agent","created_at":"2026-08-02T00:00:00Z"}
		]}`))
	}))
	defer server.Close()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.PublicAPI = client.NewPublicAPI(server.URL, nil)

	result, err := listAPITokens(deps)(callerContext(t, "user-1", "caller-token"), ListAPITokensArgs{})
	if err != nil {
		t.Fatalf("can't list tokens: %v", err)
	}

	// An MCP key lives in the same table but is not an API token, and listing
	// it here would invite the model to revoke the wrong thing.
	if result.Count != 1 || len(result.Tokens) != 1 {
		t.Fatalf("listed %d tokens, want 1", result.Count)
	}
	if result.Tokens[0].ID != "token-1" {
		t.Errorf("listed %q, want token-1", result.Tokens[0].ID)
	}
	if len(result.Tokens[0].Permissions) != 1 || result.Tokens[0].Permissions[0] != permissionEventsRead {
		t.Errorf("permissions are %v, want [events:read]", result.Tokens[0].Permissions)
	}
}

func TestDeleteAPIToken(t *testing.T) {
	t.Parallel()

	var path string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.PublicAPI = client.NewPublicAPI(server.URL, nil)

	ctx := callerContext(t, "user-1", "caller-token")

	if _, err := deleteAPIToken(deps)(ctx, DeleteAPITokenArgs{ID: "token-1"}); err != nil {
		t.Fatalf("can't delete the token: %v", err)
	}

	if path != accessTokenPathTest+"/token-1" {
		t.Errorf("deleted through %q, want %q", path, accessTokenPathTest+"/token-1")
	}

	if _, err := deleteAPIToken(deps)(ctx, DeleteAPITokenArgs{}); err == nil {
		t.Error("a delete without an identifier was accepted")
	}
}

// permissionEventsRead is the smallest permission a token can be issued with.
const permissionEventsRead = "events:read"

// accessTokenPathTest mirrors the route Public API serves tokens on, so that a
// change there fails this test rather than silently deleting nothing.
const accessTokenPathTest = "/api/v1/access-token"

// callerContext builds the context addTool hands a handler once the caller has
// been authorized.
func callerContext(t *testing.T, userID, token string) context.Context {
	t.Helper()

	authorizer, err := auth.New(false, "")
	if err != nil {
		t.Fatalf("can't build authorizer: %v", err)
	}

	ctx, _, err := authorizer.Authorize(context.Background(), token)
	if err != nil {
		t.Fatalf("can't authorize: %v", err)
	}

	return auth.WithCaller(ctx, auth.Caller{Username: "tester", UserID: userID})
}
