package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	enf_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

type (
	AdmissionEventThreats []*api.Threat
	// AdmissionResourceJSON is type over api.Resource implementing sql.Scanner and driver.Valuer
	// so that it can be saved at database as JSON object in proper way.
	AdmissionResourceJSON api.Resource
)

// AdmissionEvent represents admission event to be stored in database.
// Following RuntimeEvent, it keeps the resource itself in JSON format (SourceResource) and a set of
// flat fields that can be used for filtering.
type AdmissionEvent struct {
	// Note: we cannot use Base model for ID and CreatedAt because type and indices annotations of uuid and time in PG and ClickHouse are differs
	ID uuid.UUID `gorm:"type:UUID"`
	// Timestamp when event had been saved to database
	CreatedAt time.Time `gorm:"type:DateTime64(9)"`
	// Timestamp when event had been registered by admission monitor
	RegisteredAt time.Time `gorm:"type:DateTime64(9)"`

	KyvernoVersion string
	SourceResource *AdmissionResourceJSON `gorm:"type:String"` // Resource the policy was evaluated against. Normally should not be nil.

	ResourceAPIVersion string
	ResourceKind       string
	ResourceNamespace  string
	ResourceName       string
	ResourceUID        string
	NodeName           string
	ContainerNames     []string `gorm:"type:Array(String)"`
	ImageNames         []string `gorm:"type:Array(String)"`
	Registries         []string `gorm:"type:Array(String)"`

	Threats         AdmissionEventThreats `gorm:"type:Nullable(String)"`
	ThreatsPolicies []string              `gorm:"type:Array(String)"` // Identifiers of Kyverno policies from Threats

	// Blocked is true when Kyverno denied the request, that is the source is in the enforce mode.
	Blocked          bool
	IsIncident       bool
	IncidentSeverity enf_model.Severity
	BlockBy          []string `gorm:"type:Array(String)"`
	NotifyBy         []string `gorm:"type:Array(String)"`
}

// BeforeCreate callback sets ID as newly generated UUID. If ID was set already, just go ahead and do nothing.
// Note: we cannot use Base model for this purpose because type and indices annotations of uuid type in PG and ClickHouse are differs
func (ae *AdmissionEvent) BeforeCreate(*gorm.DB) error {
	if ae.ID == uuid.Nil {
		ae.ID = uuid.New()
	}

	return nil
}

func (t *AdmissionEventThreats) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", src)
	}

	return json.Unmarshal([]byte(s), t)
}

func (t AdmissionEventThreats) Value() (driver.Value, error) {
	if t == nil {
		return nil, nil
	}

	marshalled, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("can't marshal json: %w", err)
	}

	return string(marshalled), nil
}

func (r *AdmissionResourceJSON) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", src)
	}

	return protojson.Unmarshal([]byte(s), (*api.Resource)(r))
}

func (r *AdmissionResourceJSON) Value() (driver.Value, error) {
	b, err := protojson.Marshal((*api.Resource)(r))
	if err != nil {
		return nil, err
	}

	return string(b), nil
}
