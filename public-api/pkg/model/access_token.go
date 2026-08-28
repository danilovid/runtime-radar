package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

var (
	ErrTokenNameInUse = errors.New("token name in use")
)

type Permissions jwt.RolePermissions

// TokenKind tells apart the two things this table stores. They are deliberately
// not interchangeable: an MCP key must not open the public API, and a public
// API token must not open the MCP server.
type TokenKind string

const (
	// TokenKindAccess is a public API token, the original use of this table.
	TokenKindAccess TokenKind = "access"
	// TokenKindMCP is a key an external AI agent authenticates to MCP Server
	// with. It is exchanged for a short-lived JWT rather than used downstream.
	TokenKindMCP TokenKind = "mcp"
)

// Scopes are the halves of the product an MCP key may reach. They are a
// property of the key rather than of a role: the two monitors are guarded by
// one permission in the product, and a key narrows that further for the agent
// holding it. Only MCP Server enforces them, which is where the key is used.
//
// An empty list means the key is unrestricted, which is what every key issued
// before scopes existed is.
type Scopes []string

const (
	// ScopeRuntimeMonitor covers the runtime half: Tetragon events, the
	// detectors that flag them and their statistics.
	ScopeRuntimeMonitor = "runtime_monitor"
	// ScopeAdmission covers the admission half: the Kyverno sources and the
	// findings they produced.
	ScopeAdmission = "admission"
)

// KnownScopes are the scopes a key may be issued with.
var KnownScopes = []string{ScopeRuntimeMonitor, ScopeAdmission}

// IsKnownScope reports whether name is a scope this product understands.
func IsKnownScope(name string) bool {
	for _, scope := range KnownScopes {
		if scope == name {
			return true
		}
	}

	return false
}

func (s *Scopes) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte, got %T", src)
	}

	return json.Unmarshal(b, s)
}

func (s Scopes) Value() (driver.Value, error) {
	return json.Marshal(s)
}

type AccessToken struct {
	Base

	Name        string
	UserID      uuid.UUID
	Kind        TokenKind    `gorm:"not null;default:access;index"`
	Hash        string       `gorm:"uniqueIndex"`
	Permissions *Permissions `gorm:"type:jsonb"`
	// Scopes only ever apply to a key of kind mcp.
	Scopes        Scopes `gorm:"type:jsonb"`
	ExpiresAt     *time.Time
	InvalidatedAt *time.Time // Timestamp when the token was invalidated by an administrator. Users cannot invalidate their own tokens.
}

func (a *AccessToken) BeforeCreate(tx *gorm.DB) error {
	base := any(&a.Base)
	if b, ok := base.(callbacks.BeforeCreateInterface); ok {
		if err := b.BeforeCreate(tx); err != nil {
			return err
		}
	}

	if a.Kind == "" {
		a.Kind = TokenKindAccess
	}

	if err := a.checkTokenNameUnique(tx); err != nil {
		return err
	}

	return nil
}

func (a *AccessToken) checkTokenNameUnique(tx *gorm.DB) error {
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(&AccessToken{Name: a.Name, UserID: a.UserID, Kind: a.Kind}).
		Not(&AccessToken{Base: Base{ID: a.ID}}).
		Take(&AccessToken{}).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}

		return fmt.Errorf("can't check if token name is in use: %w", err)
	}

	return ErrTokenNameInUse
}

func (p *Permissions) Scan(src interface{}) error {
	b := src.([]byte)
	return json.Unmarshal(b, p)
}

func (p *Permissions) Value() (driver.Value, error) {
	return json.Marshal(p)
}
