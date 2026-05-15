package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/cache"
	"context"
	"fmt"
	"time"
)

// dbQueryTimeout is the per-service-method budget for all DB operations.
// It must be shorter than the HTTP server WriteTimeout so that a slow query
// fails fast and returns a clean error rather than letting the server hard-kill
// the connection mid-response.
const dbQueryTimeout = 5 * time.Second

// defaultTone is the reminder/message tone applied when the caller omits the field.
const defaultTone = "polite"

// defaultChannel is the delivery channel applied when the caller omits the field.
const defaultChannel = "line"

// withDBTimeout wraps parent with a 5 s deadline for DB calls.
// Call defer cancel() immediately after; all store calls in the method share this budget.
func withDBTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbQueryTimeout) //nolint:gosec // cancel func is returned to the caller; caller is responsible for deferring it
}

// sendPINResetEmail generates a one-time OTP, stores it in cache, and emails it to the user.
func (s *AuthService) sendPINResetEmail(ctx context.Context, userID, email, name string) error {
	otp, err := generateOTP()
	if err != nil {
		return api.ErrInternal(fmt.Errorf("generate OTP: %w", err))
	}

	if err := s.cache.StoreOTP(ctx, userID, otp); err != nil {
		return api.ErrInternal(fmt.Errorf("store OTP: %w", err))
	}

	if err := s.emailSender.SendOTP(ctx, email, name, otp); err != nil {
		return api.ErrInternal(fmt.Errorf("send PIN reset email: %w", err))
	}

	return nil
}

// cacheSession mirrors a DB session into Redis (best-effort; a cache miss falls back to DB).
func (s *AuthService) cacheSession(ctx context.Context, session db.Session) {
	cached := &cache.Session{
		ID:           session.ID.String(),
		UserID:       session.UserID.String(),
		RefreshToken: session.RefreshToken,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
		LastActiveAt: session.CreatedAt,
	}
	if session.UserAgent != nil {
		cached.UserAgent = *session.UserAgent
	}
	if session.IpAddress != nil {
		cached.IPAddress = *session.IpAddress
	}
	_ = s.cache.CreateSession(ctx, cached) //nolint:errcheck // fire-and-forget cache op
}
