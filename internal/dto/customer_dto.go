package dto

import (
	db "cashpilotx/db/sqlc"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Customer DTOs
// =============================================================================

// CreateCustomerRequest is the payload for creating a new customer.
type CreateCustomerRequest struct {
	Name    string   `json:"name"    validate:"required,min=1,max=200"`
	Phone   *string  `json:"phone"   validate:"omitempty,phone"`
	Country *string  `json:"country" validate:"omitempty,iso3166_1_alpha2"`
	Type    string   `json:"type"    validate:"required,oneof=customer supplier both"`
	Notes   *string  `json:"notes"   validate:"omitempty,max=2000"`
	Tags    []string `json:"tags"    validate:"omitempty,dive,min=1,max=50"`
}

// UpdateCustomerRequest carries optional fields for a partial update.
type UpdateCustomerRequest struct {
	Name    *string  `json:"name"    validate:"omitempty,min=1,max=200"`
	Phone   *string  `json:"phone"   validate:"omitempty,phone"`
	Country *string  `json:"country" validate:"omitempty,iso3166_1_alpha2"`
	Notes   *string  `json:"notes"   validate:"omitempty,max=2000"`
	Tags    []string `json:"tags"    validate:"omitempty,dive,min=1,max=50"`
}

// CustomerResponse is the public representation of a customer.
type CustomerResponse struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Phone        *string    `json:"phone,omitempty"`
	Country      *string    `json:"country,omitempty"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	TotalOwed    int64      `json:"total_owed"`
	TotalPaid    int64      `json:"total_paid"`
	CreditLimit  int64      `json:"credit_limit"`
	Notes        *string    `json:"notes,omitempty"`
	Tags         []string   `json:"tags"`
	LastActivity *time.Time `json:"last_activity,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ToCustomerResponse maps db.Customer to a client-facing CustomerResponse.
func ToCustomerResponse(c db.Customer) *CustomerResponse {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return &CustomerResponse{
		ID:           c.ID,
		Name:         c.Name,
		Phone:        c.Phone,
		Country:      c.Country,
		Type:         c.Type,
		Status:       c.Status,
		TotalOwed:    c.TotalOwed,
		TotalPaid:    c.TotalPaid,
		CreditLimit:  c.CreditLimit,
		Notes:        c.Notes,
		Tags:         tags,
		LastActivity: c.LastActivity,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

// =============================================================================
// Channel Config DTOs
// =============================================================================

// SetChannelActiveRequest is the payload for toggling is_active on a channel config.
type SetChannelActiveRequest struct {
	IsActive *bool `json:"is_active" validate:"required"`
}

// UpsertChannelConfigRequest is the payload for creating or updating a channel config.
// Config must be a valid JSON object representing the channel-specific settings
// (e.g. API keys, webhook tokens). It is encrypted at rest with AES-256-GCM.
type UpsertChannelConfigRequest struct {
	Channel  string          `json:"channel"   validate:"required,min=1,max=50"`
	Config   json.RawMessage `json:"config"    validate:"required" swaggertype:"primitive,object"`
	IsActive bool            `json:"is_active"`
}

// ChannelConfigResponse is the public representation of a channel config.
// The Config field (decrypted plaintext) is only populated on single-item fetches.
type ChannelConfigResponse struct {
	ID              uuid.UUID       `json:"id"`
	Channel         string          `json:"channel"`
	Config          json.RawMessage `json:"config,omitempty" swaggertype:"primitive,object"`
	ConfigVersion   int32           `json:"config_version"`
	IsActive        bool            `json:"is_active"`
	Status          string          `json:"status"`
	LastConnectedAt *time.Time      `json:"last_connected_at,omitempty"`
	LastError       *string         `json:"last_error,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// ToChannelConfigResponse maps db.ChannelConfig to a ChannelConfigResponse.
// Pass decryptedConfig as nil for list responses (avoid decrypting every row).
func ToChannelConfigResponse(c db.ChannelConfig, decryptedConfig json.RawMessage) *ChannelConfigResponse {
	return &ChannelConfigResponse{
		ID:              c.ID,
		Channel:         c.Channel,
		Config:          decryptedConfig,
		ConfigVersion:   c.ConfigVersion,
		IsActive:        c.IsActive,
		Status:          c.Status,
		LastConnectedAt: c.LastConnectedAt,
		LastError:       c.LastError,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}
