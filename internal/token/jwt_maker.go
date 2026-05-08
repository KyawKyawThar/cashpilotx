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

// Claims wraps jwt.RegisteredClaims. Everything CashPilotX needs
// (jti, sub, iss, iat, exp) is covered by the standard fields.
type Claims struct {
	jwt.RegisteredClaims
}

// NewJWTMaker returns a JWTMaker. secretKey must be at least 32 characters;
// issuer is stamped as the "iss" claim on every token (e.g. "cashpilotx").
func NewJWTMaker(secretKey, issuer string) (Maker, error) {
	if len(secretKey) < minSecretKeySize {
		return nil, fmt.Errorf("invalid key size: must be at least %d characters", minSecretKeySize)
	}
	return &JWTMaker{secretKey: secretKey, issuer: issuer}, nil
}

// CreateToken mints a signed JWT for the given operator.
func (j *JWTMaker) CreateToken(userID uuid.UUID, duration time.Duration) (string, *Payload, error) {
	payload, err := NewPayload(userID, duration)
	if err != nil {
		return "", nil, err
	}

	claims := &jwt.RegisteredClaims{
		ID:        payload.ID.String(),     // jti — blacklist key
		Subject:   payload.UserID.String(), // sub — operator identity
		Issuer:    j.issuer,
		ExpiresAt: jwt.NewNumericDate(payload.ExpiredAt),
		IssuedAt:  jwt.NewNumericDate(payload.IssuedAt),
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
		ID:        tokenID,
		UserID:    userID,
		IssuedAt:  claims.IssuedAt.Time,
		ExpiredAt: claims.ExpiresAt.Time,
	}, nil
}
