// Package token provides JWT creation and verification for the CashPilotX service.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const minSecretKeySize = 32

// JWTMaker creates and verifies JWTs signed with HMAC-SHA256.
type JWTMaker struct {
	secretKey string
	issuer    string
}

// Claims extends jwt.RegisteredClaims with CashPilotX-specific fields.
type Claims struct {
	jwt.RegisteredClaims
	ForcePinChange bool       `json:"force_pin_change"`
	LoginType      string     `json:"login_type"`
	Plan           string     `json:"plan"`
	Role           string     `json:"role"`
	TeamID         *uuid.UUID `json:"team_id,omitempty"`
	OrgOwnerID     *uuid.UUID `json:"org_owner_id,omitempty"`
}

// NewJWTMaker returns a JWTMaker. secretKey must be at least 32 characters.
func NewJWTMaker(secretKey, issuer string) (Maker, error) {
	if len(secretKey) < minSecretKeySize {
		return nil, fmt.Errorf("invalid key size: must be at least %d characters", minSecretKeySize)
	}
	return &JWTMaker{secretKey: secretKey, issuer: issuer}, nil
}

// CreateToken mints a signed JWT for the given operator.
func (j *JWTMaker) CreateToken(params CreateTokenParams) (string, *Payload, error) {
	payload, err := NewPayload(params)
	if err != nil {
		return "", nil, err
	}

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        payload.ID.String(),
			Subject:   payload.UserID.String(),
			Issuer:    j.issuer,
			ExpiresAt: jwt.NewNumericDate(payload.ExpiredAt),
			IssuedAt:  jwt.NewNumericDate(payload.IssuedAt),
		},
		ForcePinChange: payload.ForcePinChange,
		LoginType:      payload.LoginType,
		Plan:           payload.Plan,
		Role:           payload.Role,
		TeamID:         payload.TeamID,
		OrgOwnerID:     payload.OrgOwnerID,
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := jwtToken.SignedString([]byte(j.secretKey))
	if err != nil {
		return "", nil, err
	}

	return signed, payload, nil
}

// VerifyToken validates the signature and expiry, then reconstructs the Payload.
func (j *JWTMaker) VerifyToken(tokenString string) (*Payload, error) {
	keyFunc := func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(j.secretKey), nil
	}

	jwtToken, err := jwt.ParseWithClaims(tokenString, &Claims{}, keyFunc, jwt.WithLeeway(5*time.Second))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := jwtToken.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	tokenID, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrInvalidToken
	}

	return &Payload{
		ID:             tokenID,
		UserID:         userID,
		IssuedAt:       claims.IssuedAt.Time,
		ExpiredAt:      claims.ExpiresAt.Time,
		ForcePinChange: claims.ForcePinChange,
		LoginType:      claims.LoginType,
		Plan:           claims.Plan,
		Role:           claims.Role,
		TeamID:         claims.TeamID,
		OrgOwnerID:     claims.OrgOwnerID,
	}, nil
}
