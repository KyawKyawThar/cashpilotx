package token

import (
	"testing"
	"time"

	"cashpilotx/utils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newTestMaker(t *testing.T) Maker {
	t.Helper()
	maker, err := NewJWTMaker(utils.RandomString(32), "cashpilotx")
	require.NoError(t, err)
	return maker
}

func TestJWTMaker(t *testing.T) {
	maker := newTestMaker(t)
	userID := uuid.New()
	duration := time.Minute

	issuedAt := time.Now()
	expiresAt := issuedAt.Add(duration)

	token, payload, err := maker.CreateToken(CreateTokenParams{UserID: userID, Duration: duration, Plan: "free"})
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotEmpty(t, payload)

	payload, err = maker.VerifyToken(token)
	require.NoError(t, err)
	require.NotEmpty(t, payload)

	require.NotZero(t, payload.ID)
	require.Equal(t, userID, payload.UserID)
	require.WithinDuration(t, issuedAt, payload.IssuedAt, time.Second)
	require.WithinDuration(t, expiresAt, payload.ExpiredAt, time.Second)
}

func TestExpiredJWTToken(t *testing.T) {
	maker := newTestMaker(t)

	token, payload, err := maker.CreateToken(CreateTokenParams{UserID: uuid.New(), Duration: -time.Minute, Plan: "free"})
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotEmpty(t, payload)

	payload, err = maker.VerifyToken(token)
	require.Error(t, err)
	require.EqualError(t, err, ErrTokenExpired.Error())
	require.Nil(t, payload)
}

func TestInvalidJWTTokenAlgNone(t *testing.T) {
	userID := uuid.New()
	payload, err := NewPayload(CreateTokenParams{UserID: userID, Duration: time.Minute, Plan: "free"})
	require.NoError(t, err)

	claims := &jwt.RegisteredClaims{
		ID:        payload.ID.String(),
		Subject:   payload.UserID.String(),
		ExpiresAt: jwt.NewNumericDate(payload.ExpiredAt),
		IssuedAt:  jwt.NewNumericDate(payload.IssuedAt),
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	token, err := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	maker := newTestMaker(t)
	payload, err = maker.VerifyToken(token)
	require.Nil(t, payload)
	require.Error(t, err)
	require.EqualError(t, err, ErrInvalidToken.Error())
}

func TestInvalidSecretKeySize(t *testing.T) {
	maker, err := NewJWTMaker(utils.RandomString(31), "cashpilotx")
	require.Error(t, err)
	require.Nil(t, maker)
}
