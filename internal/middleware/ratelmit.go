package middleware

import (
	"cashpilotx/internal/api"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// =============================================================================
// Rate Limit Config
// =============================================================================

type RateLimitConfig struct {
	Limit           int
	Window          time.Duration
	KeyFunc         func(r *http.Request) string
	SkipFunc        func(r *http.Request) bool
	LimitHeader     string
	RemainingHeader string
	ResetHeader     string
}

// DefaultRateLimitConfig returns a 100 req/min per-IP limiter.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Limit:           100,
		Window:          time.Minute,
		KeyFunc:         func(r *http.Request) string { return r.RemoteAddr },
		LimitHeader:     "X-RateLimit-Limit",
		RemainingHeader: "X-RateLimit-Remaining",
		ResetHeader:     "X-RateLimit-Reset",
	}
}

// PerUserRateLimitConfig limits authenticated routes by user ID, falling back
// to IP for unauthenticated requests.
func PerUserRateLimitConfig(limit int, window time.Duration) RateLimitConfig {
	return RateLimitConfig{
		Limit:  limit,
		Window: window,
		KeyFunc: func(r *http.Request) string {
			if uid := GetUserID(r.Context()); uid != "" {
				return "user:" + uid
			}
			return "ip:" + r.RemoteAddr
		},
		LimitHeader:     "X-RateLimit-Limit",
		RemainingHeader: "X-RateLimit-Remaining",
		ResetHeader:     "X-RateLimit-Reset",
	}
}

// =============================================================================
// In-Memory Rate Limiter
// =============================================================================

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

type InMemoryRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateLimitEntry
	config  RateLimitConfig
}

func NewInMemoryRateLimiter(config RateLimitConfig) *InMemoryRateLimiter {
	rl := &InMemoryRateLimiter{
		entries: make(map[string]*rateLimitEntry),
		config:  config,
	}
	go rl.cleanup()
	return rl
}

func (rl *InMemoryRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.Window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, e := range rl.entries {
			if e.resetTime.Before(now) {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *InMemoryRateLimiter) Allow(key string) (allowed bool, remaining int, resetTime time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	e, exists := rl.entries[key]
	if !exists || e.resetTime.Before(now) {
		e = &rateLimitEntry{count: 1, resetTime: now.Add(rl.config.Window)}
		rl.entries[key] = e
		return true, rl.config.Limit - 1, e.resetTime
	}
	if e.count >= rl.config.Limit {
		return false, 0, e.resetTime
	}
	e.count++
	return true, rl.config.Limit - e.count, e.resetTime
}

// =============================================================================
// Rate Limit Middleware
// =============================================================================

// RateLimit creates an in-memory rate limiting middleware.
func RateLimit(config RateLimitConfig) func(http.Handler) http.Handler {
	limiter := NewInMemoryRateLimiter(config)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config.SkipFunc != nil && config.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			key := config.KeyFunc(r)
			allowed, remaining, resetTime := limiter.Allow(key)

			w.Header().Set(config.LimitHeader, fmt.Sprintf("%d", config.Limit))
			w.Header().Set(config.RemainingHeader, fmt.Sprintf("%d", remaining))
			w.Header().Set(config.ResetHeader, fmt.Sprintf("%d", resetTime.Unix()))

			if !allowed {
				retryAfter := max(1, int(time.Until(resetTime).Seconds()))
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				api.HandleError(w, r, api.ErrRateLimited(retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Redis Rate Limiter (interface for future Redis-backed implementation)
// =============================================================================

type RedisRateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time, err error)
}

// RateLimitWithRedis creates a Redis-backed rate limiting middleware.
func RateLimitWithRedis(limiter RedisRateLimiter, config RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config.SkipFunc != nil && config.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			key := config.KeyFunc(r)
			allowed, remaining, resetTime, err := limiter.Allow(r.Context(), key, config.Limit, config.Window)
			if err != nil {
				// On limiter error, fail open — allow the request
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set(config.LimitHeader, fmt.Sprintf("%d", config.Limit))
			w.Header().Set(config.RemainingHeader, fmt.Sprintf("%d", remaining))
			w.Header().Set(config.ResetHeader, fmt.Sprintf("%d", resetTime.Unix()))

			if !allowed {
				retryAfter := max(1, int(time.Until(resetTime).Seconds()))
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				api.HandleError(w, r, api.ErrRateLimited(retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
