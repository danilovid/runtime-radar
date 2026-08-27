package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/client"
)

const (
	// defaultTokenLifetimeDays is how long a token lives when the caller did
	// not say. A credential that never expires is not a sensible default for
	// something created in a conversation.
	defaultTokenLifetimeDays = 90
	// maxTokenLifetimeDays bounds the lifetime a caller may ask for.
	maxTokenLifetimeDays = 365

	// apiTokenKind is the kind of token these tools manage. MCP keys live in
	// the same table but are issued and listed on their own routes.
	apiTokenKind = "access"

	// secretHandlingNote travels with every secret this server returns. Clients
	// differ in what they do with a tool result, so the expectation is spelled
	// out in the result itself as well as in the tool's description.
	secretHandlingNote = "Show this value to the user once, exactly as it is. Do not repeat it in later messages, " +
		"do not write it to a file, and do not send it anywhere: it is a credential to the product's API and it " +
		"cannot be read again."
)

// tokenResources are the resources an API token can be scoped to, spelled the
// way the product's own API spells them, so that what a user reads in the UI
// and what they ask the agent for are the same words. They are also the JSON
// field names of a role's permissions, which is how a permission set is built
// and read back below.
var tokenResources = []string{
	"users",
	"roles",
	"rules",
	"scopes",
	"scanning",
	"events",
	"registries",
	"images",
	"integrations",
	"notifications",
	"system_settings",
	"clusters",
	"public_access_tokens",
	"invalidate_public_access_tokens",
}

// tokenActions are the actions a permission may grant.
var tokenActions = map[string]jwt.Action{
	string(jwt.ActionCreate):  jwt.ActionCreate,
	string(jwt.ActionRead):    jwt.ActionRead,
	string(jwt.ActionUpdate):  jwt.ActionUpdate,
	string(jwt.ActionDelete):  jwt.ActionDelete,
	string(jwt.ActionExecute): jwt.ActionExecute,
}

// APITokenPermission is one resource an API token may be used on.
type APITokenPermission struct {
	Resource string   `json:"resource" jsonschema:"resource the token may be used on: users, roles, rules, scopes, scanning, events, registries, images, integrations, notifications, system_settings, clusters, public_access_tokens or invalidate_public_access_tokens"`
	Actions  []string `json:"actions" jsonschema:"actions allowed on that resource: create, read, update, delete or execute"`
}

// APITokenInfo is one API token, without its secret.
type APITokenInfo struct {
	ID            string   `json:"id" jsonschema:"identifier of the token. Pass it to delete_api_token"`
	Name          string   `json:"name" jsonschema:"name the token was created under"`
	Permissions   []string `json:"permissions,omitempty" jsonschema:"what the token may do, as resource:action pairs"`
	CreatedAt     string   `json:"created_at,omitempty" jsonschema:"when the token was created, RFC3339"`
	ExpiresAt     string   `json:"expires_at,omitempty" jsonschema:"when the token expires, RFC3339. Empty when it never does"`
	InvalidatedAt string   `json:"invalidated_at,omitempty" jsonschema:"when an administrator revoked the token, RFC3339. Empty while it is valid"`
}

// ListAPITokensArgs are the arguments of list_api_tokens.
type ListAPITokensArgs struct {
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring to filter tokens by name"`
}

// ListAPITokensResult is the answer of list_api_tokens.
type ListAPITokensResult struct {
	Tokens []APITokenInfo `json:"tokens" jsonschema:"API tokens belonging to the user this session authenticated as"`
	Count  int            `json:"count" jsonschema:"number of tokens returned"`
	Total  int            `json:"total" jsonschema:"number of tokens the user has, before the search filter was applied"`
}

// CreateAPITokenArgs are the arguments of create_api_token.
type CreateAPITokenArgs struct {
	Name          string               `json:"name" jsonschema:"name of the token, so that the user can recognise it later. Required"`
	Permissions   []APITokenPermission `json:"permissions" jsonschema:"what the token may do. Required, and kept as small as the stated purpose allows: a token is a lasting credential"`
	ExpiresInDays int                  `json:"expires_in_days,omitempty" jsonschema:"how many days the token stays valid, at most 365. Defaults to 90"`
}

// SecretValue is a credential this server produced. It is returned once and
// cannot be read again, which is why it travels in a field of its own rather
// than inside prose the client may summarise away.
type SecretValue struct {
	Label string `json:"label" jsonschema:"what this secret is"`
	Value string `json:"value" jsonschema:"the secret itself, shown once"`
	Note  string `json:"note" jsonschema:"how the secret is to be handled"`
}

// CreateAPITokenResult is the answer of create_api_token.
type CreateAPITokenResult struct {
	ID          string       `json:"id" jsonschema:"identifier of the token that was created"`
	Name        string       `json:"name" jsonschema:"name of the token that was created"`
	ExpiresAt   string       `json:"expires_at" jsonschema:"when the token expires, RFC3339"`
	Permissions []string     `json:"permissions" jsonschema:"what the token may do, as resource:action pairs"`
	Secret      *SecretValue `json:"secret" jsonschema:"the token itself. It is shown once and cannot be recovered"`
}

// DeleteAPITokenArgs are the arguments of delete_api_token.
type DeleteAPITokenArgs struct {
	ID string `json:"id" jsonschema:"identifier of the token to delete, as returned by list_api_tokens. Required"`
}

// DeleteAPITokenResult is the answer of delete_api_token.
type DeleteAPITokenResult struct {
	ID      string `json:"id" jsonschema:"identifier of the token that was deleted"`
	Deleted bool   `json:"deleted" jsonschema:"always true: a token that could not be deleted is reported as an error instead"`
}

func registerTokenTools(server *mcp.Server, deps *Deps) {
	addTool(server, deps, &mcp.Tool{
		Name:        "list_api_tokens",
		Annotations: readOnly("List API tokens"),
		Description: "List the product's API tokens belonging to the user this session authenticated as: name, " +
			"permissions, when they were created and when they expire. Secrets are not listed — a token's value " +
			"exists only in the answer that created it.",
	}, []auth.Permission{auth.ReadAPITokens()}, listAPITokens(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "create_api_token",
		Annotations: write("Create an API token", false),
		Description: "Create a token for the product's public API, belonging to the user this session " +
			"authenticated as. The token can do only what its permissions allow and only what that user's role " +
			"already allows, whichever is narrower. Ask the user what the token is for, propose the smallest set " +
			"of permissions that covers it, and create it only once they agreed. The answer contains the secret; " +
			"show it to the user once and never repeat it.",
	}, []auth.Permission{auth.CreateAPITokens()}, createAPIToken(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "delete_api_token",
		Annotations: write("Delete an API token", true),
		Description: "Delete one of the user's API tokens by its identifier. Whatever authenticates with it stops " +
			"working immediately, so name the token to the user and get their agreement before calling this.",
	}, []auth.Permission{auth.DeleteAPITokens()}, deleteAPIToken(deps))
}

func listAPITokens(deps *Deps) func(context.Context, ListAPITokensArgs) (ListAPITokensResult, error) {
	return func(ctx context.Context, args ListAPITokensArgs) (ListAPITokensResult, error) {
		tokens, total, err := deps.PublicAPI.ListAPITokens(ctx, auth.BearerFromContext(ctx))
		if err != nil {
			return ListAPITokensResult{}, fmt.Errorf("can't list api tokens: %w", err)
		}

		search := strings.ToLower(strings.TrimSpace(args.Search))

		result := ListAPITokensResult{Tokens: make([]APITokenInfo, 0, len(tokens)), Total: total}

		for _, token := range tokens {
			if token.Kind != "" && token.Kind != apiTokenKind {
				continue
			}

			if search != "" && !strings.Contains(strings.ToLower(token.Name), search) {
				continue
			}

			result.Tokens = append(result.Tokens, tokenInfo(token))
		}

		result.Count = len(result.Tokens)

		return result, nil
	}
}

func createAPIToken(deps *Deps) func(context.Context, CreateAPITokenArgs) (CreateAPITokenResult, error) {
	return func(ctx context.Context, args CreateAPITokenArgs) (CreateAPITokenResult, error) {
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return CreateAPITokenResult{}, fmt.Errorf("name is required")
		}

		caller := auth.CallerFromContext(ctx)
		if caller.UserID == "" {
			// Public API issues a token to a user, and the only user this
			// server may act for is the one who authenticated the session.
			return CreateAPITokenResult{}, fmt.Errorf("this session is not authenticated as a user, so no token can be issued")
		}

		permissions, granted, err := args.rolePermissions()
		if err != nil {
			return CreateAPITokenResult{}, err
		}

		expiresAt, err := args.expiry(deps.now())
		if err != nil {
			return CreateAPITokenResult{}, err
		}

		resp, err := deps.PublicAPI.CreateAPIToken(ctx, auth.BearerFromContext(ctx), &client.CreateAPITokenReq{
			Name:        name,
			UserID:      caller.UserID,
			ExpiresAt:   &expiresAt,
			Permissions: permissions,
		})
		if err != nil {
			return CreateAPITokenResult{}, fmt.Errorf("can't create the api token: %w", err)
		}

		return CreateAPITokenResult{
			ID:          resp.ID,
			Name:        name,
			ExpiresAt:   expiresAt.Format(time.RFC3339),
			Permissions: granted,
			Secret: &SecretValue{
				Label: "API token " + name,
				Value: resp.AccessToken,
				Note:  secretHandlingNote,
			},
		}, nil
	}
}

func deleteAPIToken(deps *Deps) func(context.Context, DeleteAPITokenArgs) (DeleteAPITokenResult, error) {
	return func(ctx context.Context, args DeleteAPITokenArgs) (DeleteAPITokenResult, error) {
		id := strings.TrimSpace(args.ID)
		if id == "" {
			return DeleteAPITokenResult{}, fmt.Errorf("id is required")
		}

		if err := deps.PublicAPI.DeleteAPIToken(ctx, auth.BearerFromContext(ctx), id); err != nil {
			return DeleteAPITokenResult{}, fmt.Errorf("can't delete the api token: %w", err)
		}

		return DeleteAPITokenResult{ID: id, Deleted: true}, nil
	}
}

// rolePermissions turns the requested resources into the permission set Public
// API stores, and returns the same set rendered as resource:action pairs so
// that what was granted can be reported back without decoding it again.
func (a CreateAPITokenArgs) rolePermissions() (*jwt.RolePermissions, []string, error) {
	if len(a.Permissions) == 0 {
		return nil, nil, fmt.Errorf("permissions are required: a token with none can do nothing")
	}

	byResource := make(map[string]*jwt.Permission, len(a.Permissions))
	granted := make([]string, 0, len(a.Permissions))

	for _, requested := range a.Permissions {
		resource := strings.ToLower(strings.TrimSpace(requested.Resource))

		if !slices.Contains(tokenResources, resource) {
			return nil, nil, fmt.Errorf("%q is not a resource an api token can be scoped to", requested.Resource)
		}

		if len(requested.Actions) == 0 {
			return nil, nil, fmt.Errorf("no actions given for %s", resource)
		}

		actions := make(jwt.Actions, 0, len(requested.Actions))

		for _, name := range requested.Actions {
			action, ok := tokenActions[strings.ToLower(strings.TrimSpace(name))]
			if !ok {
				return nil, nil, fmt.Errorf("%q is not an action, expected create, read, update, delete or execute", name)
			}

			actions = append(actions, action)
			granted = append(granted, resource+":"+string(action))
		}

		byResource[resource] = &jwt.Permission{Actions: actions}
	}

	permissions, err := rolePermissionsOf(byResource)
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(granted)

	return permissions, granted, nil
}

// rolePermissionsOf assembles a permission set from the resources it grants.
// The resource names are the JSON field names of jwt.RolePermissions, so the
// two shapes convert into one another without a table that would have to be
// kept in step with the library.
func rolePermissionsOf(byResource map[string]*jwt.Permission) (*jwt.RolePermissions, error) {
	encoded, err := json.Marshal(byResource)
	if err != nil {
		return nil, fmt.Errorf("can't encode the permissions: %w", err)
	}

	permissions := &jwt.RolePermissions{}
	if err := json.Unmarshal(encoded, permissions); err != nil {
		return nil, fmt.Errorf("can't build the permissions: %w", err)
	}

	return permissions, nil
}

// expiry resolves how long the token lives, refusing a lifetime longer than the
// product allows rather than silently shortening it.
func (a CreateAPITokenArgs) expiry(now time.Time) (time.Time, error) {
	days := a.ExpiresInDays
	if days == 0 {
		days = defaultTokenLifetimeDays
	}

	if days < 1 || days > maxTokenLifetimeDays {
		return time.Time{}, fmt.Errorf("expires_in_days must be between 1 and %d, got %d", maxTokenLifetimeDays, days)
	}

	return now.AddDate(0, 0, days), nil
}

func tokenInfo(token *client.APIToken) APITokenInfo {
	info := APITokenInfo{
		ID:          token.ID,
		Name:        token.Name,
		Permissions: permissionPairs(token.Permissions),
		CreatedAt:   token.CreatedAt.Format(time.RFC3339),
	}

	if token.ExpiresAt != nil {
		info.ExpiresAt = token.ExpiresAt.Format(time.RFC3339)
	}

	if token.InvalidatedAt != nil {
		info.InvalidatedAt = token.InvalidatedAt.Format(time.RFC3339)
	}

	return info
}

// permissionPairs renders a stored permission set as resource:action pairs.
func permissionPairs(permissions *jwt.RolePermissions) []string {
	if permissions == nil {
		return nil
	}

	encoded, err := json.Marshal(permissions)
	if err != nil {
		return nil
	}

	byResource := map[string]*jwt.Permission{}
	if err := json.Unmarshal(encoded, &byResource); err != nil {
		return nil
	}

	pairs := []string{}

	for resource, permission := range byResource {
		if permission == nil {
			continue
		}

		for _, action := range permission.Actions {
			pairs = append(pairs, resource+":"+string(action))
		}
	}

	sort.Strings(pairs)

	return pairs
}
