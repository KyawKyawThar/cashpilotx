package token

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidToken is returned when a token is invalid.
	ErrInvalidToken = errors.New("token is invalid")
	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token is expired")
)

// Payload holds the claims embedded in every CashPilotX JWT.
// ID (jti) identifies the token itself — used as the Redis blacklist key on logout.
// UserID (sub) identifies the operator.
type Payload struct {
	ID        uuid.UUID `json:"id"`      // jti — blacklist key
	UserID    uuid.UUID `json:"user_id"` // sub — operator identity
	IssuedAt  time.Time `json:"issued_at"`
	ExpiredAt time.Time `json:"expired_at"`
}

// NewPayload creates a new token payload for the given operator.
func NewPayload(userID uuid.UUID, duration time.Duration) (*Payload, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	return &Payload{
		ID:        tokenID,
		UserID:    userID,
		IssuedAt:  time.Now(),
		ExpiredAt: time.Now().Add(duration),
	}, nil
}
