package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"context"
	"time"
)

// dbQueryTimeout is the per-service-method budget for all DB operations.
// It must be shorter than the HTTP server WriteTimeout so that a slow query
// fails fast and returns a clean error rather than letting the server hard-kill
// the connection mid-response.
const dbQueryTimeout = 5 * time.Second

// withDBTimeout wraps parent with a 5 s deadline for DB calls.
// Call defer cancel() immediately after; all store calls in the method share this budget.
func withDBTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbQueryTimeout) //nolint:gosec // cancel func is returned to the caller; caller is responsible for deferring it
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
