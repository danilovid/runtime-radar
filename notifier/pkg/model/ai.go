package model

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

type AIProvider string

const (
	AIProviderOpenAICompatible AIProvider = "openai-compatible"
	AIProviderAnthropic        AIProvider = "anthropic"
	AIProviderOllama           AIProvider = "ollama"
)

type AI struct {
	Base
	Name            string `gorm:"index"`
	Provider        AIProvider
	BaseURL         string
	Model           string
	EncryptedAPIKey string
	APIKey          string `gorm:"-"`
	IsLocal         bool   `gorm:"not null;default:false"`
	Insecure        bool   `gorm:"not null;default:false"`
	CA              string
	Notifications   []*Notification `gorm:"polymorphic:Integration;polymorphicValue:ai"`
	DeletedAt       gorm.DeletedAt  `gorm:"index"`
	Meta            IntegrationMeta `gorm:"-"`
}

func (a *AI) BeforeCreate(tx *gorm.DB) error {
	base := any(&a.Base)
	if b, ok := base.(callbacks.BeforeCreateInterface); ok {
		if err := b.BeforeCreate(tx); err != nil {
			return err
		}
	}

	if a.Name != "" {
		if err := a.checkNameUnique(tx, a.Name); err != nil {
			return err
		}
	}

	return nil
}

func (a *AI) BeforeUpdate(tx *gorm.DB) error {
	base := any(&a.Base)
	if b, ok := base.(callbacks.BeforeUpdateInterface); ok {
		if err := b.BeforeUpdate(tx); err != nil {
			return err
		}
	}

	name, ok := getUpdateMapValue[string](tx, "Name")
	if ok && name != "" {
		if err := a.checkNameUnique(tx, name); err != nil {
			return err
		}
	}

	return nil
}

func (a *AI) AfterDelete(tx *gorm.DB) error {
	base := any(&a.Base)
	if b, ok := base.(callbacks.AfterDeleteInterface); ok {
		if err := b.AfterDelete(tx); err != nil {
			return err
		}
	}

	return tx.Where(&Notification{IntegrationID: a.ID}).
		Delete(&Notification{}).
		Error
}

func (a *AI) checkNameUnique(tx *gorm.DB, name string) error {
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(&AI{Name: name}).
		Not(&AI{Base: Base{ID: a.ID}}).
		Take(&AI{}).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}

		return fmt.Errorf("can't check if names are in use: %w", err)
	}

	return ErrIntegrationNameInUse
}

func (a *AI) GetID() uuid.UUID {
	return a.ID
}

func (a *AI) EncryptSensitive(c cipher.Crypter) {
	if a.APIKey == "" {
		return
	}

	a.EncryptedAPIKey = c.EncryptStringAsHex(a.APIKey)
}

func (a *AI) DecryptSensitive(c cipher.Crypter) {
	if a.EncryptedAPIKey == "" || a.APIKey != "" {
		return
	}

	a.APIKey = c.DecryptHexAsString(a.EncryptedAPIKey)
}

func (a *AI) MaskSensitive() {
	const mask = "********"
	a.EncryptedAPIKey = mask
	a.APIKey = mask
}

func (a *AI) SetMeta(m IntegrationMeta) {
	a.Meta = m
}
