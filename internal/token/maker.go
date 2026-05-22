package token

import (
	"time"

	"github.com/google/uuid"
)

// CreateTokenParams groups the fields needed to mint a new JWT.
type CreateTokenParams struct {
	UserID         uuid.UUID
	Duration       time.Duration
	ForcePinChange bool
	LoginType      string     // "phone" | "username"
	Plan           string     // "free" | "pro" | "enterprise"
	Role           string     // "super_admin" | "admin" | "accountant" | "cs_team" | "sales"
	TeamID         *uuid.UUID // nil for super_admin / admin without team
	OrgOwnerID     *uuid.UUID // super_admin's id
}

// Maker is the interface for creating and verifying JWTs.
type Maker interface {
	CreateToken(params CreateTokenParams) (string, *Payload, error)
	VerifyToken(tokenString string) (*Payload, error)
}
