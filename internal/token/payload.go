package token

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("token is invalid")
	ErrTokenExpired = errors.New("token is expired")
)

// Payload holds the claims embedded in every CashPilotX JWT.
// ID (jti) is the Redis blacklist key on logout.
// UserID (sub) identifies the operator.
type Payload struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	IssuedAt       time.Time  `json:"issued_at"`
	ExpiredAt      time.Time  `json:"expired_at"`
	ForcePinChange bool       `json:"force_pin_change"`
	LoginType      string     `json:"login_type"`   // "phone" | "username"
	Plan           string     `json:"plan"`         // "free" | "pro" | "enterprise"
	Role           string     `json:"role"`         // "super_admin" | "admin" | "accountant" | "cs_team" | "sales"
	TeamID         *uuid.UUID `json:"team_id"`      // nil for super_admin / admin without a team
	OrgOwnerID     *uuid.UUID `json:"org_owner_id"` // super_admin's id — same for all org members
}

func NewPayload(params CreateTokenParams) (*Payload, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	return &Payload{
		ID:             tokenID,
		UserID:         params.UserID,
		IssuedAt:       time.Now(),
		ExpiredAt:      time.Now().Add(params.Duration),
		ForcePinChange: params.ForcePinChange,
		LoginType:      params.LoginType,
		Plan:           params.Plan,
		Role:           params.Role,
		TeamID:         params.TeamID,
		OrgOwnerID:     params.OrgOwnerID,
	}, nil
}
