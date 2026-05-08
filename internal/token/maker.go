package token

import (
	"time"

	"github.com/google/uuid"
)

// Maker is the interface for creating and verifying JWTs.
type Maker interface {
	CreateToken(userID uuid.UUID, duration time.Duration) (string, *Payload, error)
	VerifyToken(tokenString string) (*Payload, error)
}
