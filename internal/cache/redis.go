// Package cache provides a Redis-backed cache client for the CashPilotX service.
package cache

import (
	"cashpilotx/internal/config"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var (
	// ErrSessionNotFound is returned when a session is not found in the cache.
	ErrSessionNotFound = errors.New("session not found")
	// ErrTokenNotFound is returned when a token is not found in the cache.
	ErrTokenNotFound = errors.New("token not found")
	// ErrKeyNotFound is returned when a key is not found in the cache.
	ErrKeyNotFound = errors.New("key not found")
)

const (
	blacklistPrefix = "blacklist"
	rateLimitPrefix = "ratelimit"

	// =============================================================================
	// Session Management
	// =============================================================================
	sessionPrefix     = "session"
	userSessionPrefix = "user_sessions"

	// =============================================================================
	// Password Reset Tokens
	// =============================================================================

	resetTokenPrefix = "reset_token"
	resetTokenTTL    = 1 * time.Hour

	// =============================================================================
	// PIN Reset OTP (keyed by userID)
	// =============================================================================

	otpResetPrefix = "otp:reset"
	otpResetTTL    = 10 * time.Minute

	// =============================================================================
	// Registration Email OTP (keyed by email)
	// =============================================================================

	otpRegPrefix = "otp:reg"
	otpRegTTL    = 10 * time.Minute

	// =============================================================================
	// Email Verification Tokens
	// =============================================================================

	verifyEmailPrefix = "verify_email"
	verifyEmailTTL    = 10 * time.Minute

	// =============================================================================
	// Login Attempt Tracking (Brute Force Protection)
	// =============================================================================

	loginAttemptPrefix   = "login_attempts"
	loginAttemptWindow   = 15 * time.Minute
	maxLoginAttempts     = 5
	loginLockoutDuration = 30 * time.Minute
)

// RedisClient is a wrapper around the go-redis client
type RedisClient struct {
	Client *redis.Client
	Prefix string // Key prefix for namespacing (e.g., "cpx:")
}

// RedisConfig is the configuration for the Redis client.
type RedisConfig struct {
	// Address may contain a full redis URI (redis:// or rediss://) or a
	// simple host:port pair. If provided, it takes precedence over Host/Port
	// fields and is parsed internally by NewRedisClient.
	Address  string
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
	TLS      bool
}

func parseSimpleRedisAddr(urlStr string, port int) (RedisConfig, error) {

	cfg := RedisConfig{Port: port}
	host := urlStr

	if parts := strings.Split(urlStr, ":"); len(parts) == 2 {
		host = parts[0]

		if p, err := strconv.Atoi(parts[1]); err == nil {
			cfg.Port = p
		}
	}

	cfg.Host = host

	return cfg, nil
}

func parseRedisURL(urlStr string, port int) (RedisConfig, error) {

	cfg := RedisConfig{Port: port}

	u, err := url.Parse(urlStr)

	if err != nil {
		return cfg, fmt.Errorf("invalid redis url: %w", err)
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return cfg, fmt.Errorf("invalid redis scheme: %s", u.Scheme)
	}

	cfg.TLS = u.Scheme == "rediss"

	cfg.Host = u.Hostname()

	if p := u.Port(); p != "" {
		if pi, err := strconv.Atoi(p); err == nil {
			cfg.Port = pi
		}
	}

	if u.User != nil {
		if pass, ok := u.User.Password(); ok {
			cfg.Password = pass
		}
	}

	if u.Path != "" && u.Path != "/" {
		dbStr := strings.TrimPrefix(u.Path, "/")

		if di, err := strconv.Atoi(dbStr); err == nil {
			cfg.DB = di
		}
	}

	return cfg, nil
}

// ParseRedisConfig takes either a simple host:port string or a full Redis URL
// (e.g. redis://:password@host:6379/0) and returns a RedisConfig that can be
// passed to NewRedisClient. The returned config fills Host, Port, Password and
// DB fields. Returns an error if the input string is empty.
func ParseRedisConfig(urlStr string) (RedisConfig, error) {

	cfg, err := config.LoadConfig("../../")

	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")

	}
	if urlStr == "" {
		return RedisConfig{}, fmt.Errorf("empty redis url")
	}

	if strings.HasPrefix(urlStr, "redis://") || strings.HasPrefix(urlStr, "rediss://") {
		return parseRedisURL(urlStr, cfg.RedisPort)
	}

	return parseSimpleRedisAddr(urlStr, cfg.RedisPort)
}

// buildOptsFromConfig builds redis.Options from a non-URL RedisConfig.
func buildOptsFromConfig(cfg RedisConfig, port int) (*redis.Options, error) {
	if cfg.Address != "" {
		parsed, err := ParseRedisConfig(cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid redis address: %w", err)
		}
		if cfg.Host == "" {
			cfg.Host = parsed.Host
		}
		if cfg.Port == 0 {
			cfg.Port = parsed.Port
		}
		if cfg.Password == "" {
			cfg.Password = parsed.Password
		}
		if cfg.DB == 0 {
			cfg.DB = parsed.DB
		}
	}
	if cfg.Port == 0 {
		cfg.Port = port
	}
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
			ServerName:         cfg.Host,
		}
	}
	return opts, nil
}

// NewRedisClient creates a new Redis client and verifies the connection with Ping.
// Default key prefix is "cpx:" if none is provided in cfg.
func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {

	conf, err := config.LoadConfig("../../")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")

	}
	var opts *redis.Options

	if cfg.Address != "" && (strings.HasPrefix(cfg.Address, "redis://") || strings.HasPrefix(cfg.Address, "rediss://")) {
		var err error
		opts, err = redis.ParseURL(cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid redis URL: %w", err)
		}
	} else {
		var err error
		opts, err = buildOptsFromConfig(cfg, conf.RedisPort)
		if err != nil {
			return nil, err
		}
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "cpx:" // CashPilotX namespace
	}
	return &RedisClient{
		Client: client,
		Prefix: prefix,
	}, nil
}

// Close closes the Redis client.
func (r *RedisClient) Close() error {
	return r.Client.Close()
}

// GetClient returns the underlying redis client (for testing).
func (r *RedisClient) GetClient() *redis.Client {
	return r.Client
}

// Key generates a prefixed key for Redis.
// Example: r.Key("session", "abc") → "cpx:session:abc"
func (r *RedisClient) Key(parts ...string) string {
	result := r.Prefix
	for _, part := range parts {
		result += part + ":"
	}
	return result[:len(result)-1] // Remove trailing colon
}

// =============================================================================
// Generic Key-Value Ops
// =============================================================================

// Set stores a JSON-marshaled value in Redis with an expiration.
func (r *RedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.Client.Set(ctx, r.Key(key), data, expiration).Err()
}

// Get retrieves a JSON value from Redis and unmarshals it into dest.
func (r *RedisClient) Get(ctx context.Context, key string, dest any) error {
	data, err := r.Client.Get(ctx, r.Key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrKeyNotFound
		}
		return err
	}
	return json.Unmarshal(data, dest)
}

// GetString retrieves a plain string value from Redis.
func (r *RedisClient) GetString(ctx context.Context, key string) (string, error) {
	result, err := r.Client.Get(ctx, r.Key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrKeyNotFound
	}
	return result, err
}

// SetString stores a plain string value with expiration.
func (r *RedisClient) SetString(ctx context.Context, key, value string, expiration time.Duration) error {
	return r.Client.Set(ctx, r.Key(key), value, expiration).Err()
}

// Delete removes one or more keys from Redis.
func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = r.Key(k)
	}
	return r.Client.Del(ctx, prefixedKeys...).Err()
}

// Exists checks whether a key exists in Redis.
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.Client.Exists(ctx, r.Key(key)).Result()
	return result > 0, err
}

// Expire sets an expiration on a key.
func (r *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.Client.Expire(ctx, r.Key(key), expiration).Err()
}

// TTL returns the remaining time to live of a key.
func (r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.Client.TTL(ctx, r.Key(key)).Result()
}

// Session represents an operator session. CashPilotX is single-user per account
// so there is no TenantID — UserID alone identifies the operator.
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// CreateSession stores a new session in Redis indexed by session ID and user ID.
func (r *RedisClient) CreateSession(ctx context.Context, session *Session) error {
	sessionKey := r.Key(sessionPrefix, session.ID)
	data, err := json.Marshal(session) //nolint:gosec // not a credential
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session expired")
	}
	if err := r.Client.Set(ctx, sessionKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	// Track session ID in the user's session set for list/revoke-all queries.
	userSessionsKey := r.Key(userSessionPrefix, session.UserID)
	if err := r.Client.SAdd(ctx, userSessionsKey, session.ID).Err(); err != nil {
		return fmt.Errorf("failed to add session to user set: %w", err)
	}
	r.Client.Expire(ctx, userSessionsKey, ttl+time.Hour)

	return nil
}

// GetSession retrieves a session by ID.
func (r *RedisClient) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := r.Key(sessionPrefix, sessionID)
	data, err := r.Client.Get(ctx, sessionKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// UpdateSessionActivity refreshes the LastActiveAt timestamp.
func (r *RedisClient) UpdateSessionActivity(ctx context.Context, sessionID string) error {
	session, err := r.GetSession(ctx, sessionID)
	if err != nil || session == nil {
		return err
	}

	session.LastActiveAt = time.Now().UTC()
	sessionKey := r.Key(sessionPrefix, sessionID)
	data, err := json.Marshal(session) //nolint:gosec // not a credential
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	return r.Client.Set(ctx, sessionKey, data, ttl).Err()
}

// DeleteSession removes a single session and cleans up the user's session set.
func (r *RedisClient) DeleteSession(ctx context.Context, sessionID, userID string) error {
	sessionKey := r.Key(sessionPrefix, sessionID)
	userSessionsKey := r.Key(userSessionPrefix, userID)

	pipe := r.Client.Pipeline()
	pipe.Del(ctx, sessionKey)
	pipe.SRem(ctx, userSessionsKey, sessionID)
	_, err := pipe.Exec(ctx)
	return err
}

// RevokeSession is an alias for DeleteSession with parameters in (userID, sessionID) order.
func (r *RedisClient) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return r.DeleteSession(ctx, sessionID, userID)
}

// RevokeAllSessions revokes all sessions for a user (logout everywhere).
func (r *RedisClient) RevokeAllSessions(ctx context.Context, userID string) error {
	return r.DeleteAllUserSessions(ctx, userID)
}

// DeleteAllUserSessions removes every session for a user.
func (r *RedisClient) DeleteAllUserSessions(ctx context.Context, userID string) error {
	userSessionsKey := r.Key(userSessionPrefix, userID)

	sessionIDs, err := r.Client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}
	if len(sessionIDs) == 0 {
		return nil
	}

	pipe := r.Client.Pipeline()
	for _, sid := range sessionIDs {
		pipe.Del(ctx, r.Key(sessionPrefix, sid))
	}
	pipe.Del(ctx, userSessionsKey)
	_, err = pipe.Exec(ctx)
	return err
}

// GetUserSessions returns all active sessions for a user.
func (r *RedisClient) GetUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	userSessionsKey := r.Key(userSessionPrefix, userID)

	sessionIDs, err := r.Client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user session IDs: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		session, err := r.GetSession(ctx, sid)
		if err != nil {
			// Session expired or missing — remove stale ID from the set so it
			// doesn't accumulate indefinitely.
			r.Client.SRem(ctx, userSessionsKey, sid)
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// BlacklistToken adds a JWT token ID to the blacklist until it naturally expires.
// This is used during logout to invalidate short-lived JWTs before they expire.
func (r *RedisClient) BlacklistToken(ctx context.Context, tokenID string, expiresAt time.Time) error {
	key := r.Key(blacklistPrefix, tokenID)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // Already expired — no need to blacklist
	}
	return r.Client.Set(ctx, key, "1", ttl).Err()
}

// IsTokenBlacklisted reports whether a token ID is on the blacklist.
func (r *RedisClient) IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := r.Key(blacklistPrefix, tokenID)
	exists, err := r.Client.Exists(ctx, key).Result()
	return exists > 0, err
}

// ResetToken represents a password reset token.
type ResetToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StoreResetToken persists a password reset token for resetTokenTTL (1 hour).
func (r *RedisClient) StoreResetToken(ctx context.Context, token *ResetToken) error {
	key := r.Key(resetTokenPrefix, token.Token)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal reset token: %w", err)
	}
	return r.Client.Set(ctx, key, data, resetTokenTTL).Err()
}

// GetResetToken retrieves a password reset token. Returns ErrTokenNotFound when absent.
func (r *RedisClient) GetResetToken(ctx context.Context, token string) (*ResetToken, error) {
	key := r.Key(resetTokenPrefix, token)
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	var resetToken ResetToken
	if err := json.Unmarshal(data, &resetToken); err != nil {
		return nil, err
	}
	return &resetToken, nil
}

// DeleteResetToken removes a reset token after it has been consumed.
func (r *RedisClient) DeleteResetToken(ctx context.Context, token string) error {
	key := r.Key(resetTokenPrefix, token)
	return r.Client.Del(ctx, key).Err()
}

// VerifyEmailToken represents a 6-digit email verification code.
type VerifyEmailToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreVerifyEmailToken stores a verification code keyed by email address.
func (r *RedisClient) StoreVerifyEmailToken(ctx context.Context, token *VerifyEmailToken) error {
	key := r.Key(verifyEmailPrefix, token.Email)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal verify email token: %w", err)
	}
	return r.Client.Set(ctx, key, data, verifyEmailTTL).Err()
}

// GetVerifyEmailToken retrieves a verification code by email.
func (r *RedisClient) GetVerifyEmailToken(ctx context.Context, email string) (*VerifyEmailToken, error) {
	key := r.Key(verifyEmailPrefix, email)
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	var verifyToken VerifyEmailToken
	if err := json.Unmarshal(data, &verifyToken); err != nil {
		return nil, err
	}
	return &verifyToken, nil
}

// DeleteVerifyEmailToken removes a verification code after it has been consumed.
func (r *RedisClient) DeleteVerifyEmailToken(ctx context.Context, email string) error {
	key := r.Key(verifyEmailPrefix, email)
	return r.Client.Del(ctx, key).Err()
}

// RateLimitResult holds the outcome of a rate-limit check.
type RateLimitResult struct {
	Allowed    bool
	Remaining  int64
	ResetAt    time.Time
	RetryAfter int64 // seconds until the window resets
}

// CheckRateLimit implements sliding-window rate limiting via a sorted set.
func (r *RedisClient) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (*RateLimitResult, error) {
	now := time.Now()
	windowStart := now.Add(-window)
	redisKey := r.Key(rateLimitPrefix, key)

	pipe := r.Client.Pipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	countCmd := pipe.ZCard(ctx, redisKey)
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	pipe.Expire(ctx, redisKey, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	count := countCmd.Val()
	remaining := max(limit-count-1, 0)

	result := &RateLimitResult{
		Allowed:   count < limit,
		Remaining: remaining,
		ResetAt:   now.Add(window),
	}
	if !result.Allowed {
		result.RetryAfter = int64(window.Seconds())
	}
	return result, nil
}

// LoginAttemptResult is the outcome of recording a login attempt.
type LoginAttemptResult struct {
	Allowed         bool
	AttemptsLeft    int
	LockedUntil     *time.Time
	LockoutDuration time.Duration
}

// RecordLoginAttempt tracks failed login attempts and triggers a lockout after
// maxLoginAttempts failures within loginAttemptWindow.
func (r *RedisClient) RecordLoginAttempt(ctx context.Context, identifier string, success bool) (*LoginAttemptResult, error) {
	key := r.Key(loginAttemptPrefix, identifier)

	if success {
		r.Client.Del(ctx, key)
		return &LoginAttemptResult{Allowed: true, AttemptsLeft: maxLoginAttempts}, nil
	}

	count, err := r.Client.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		r.Client.Expire(ctx, key, loginAttemptWindow)
	}

	attemptsLeft := maxLoginAttempts - int(count)
	if attemptsLeft < 0 {
		attemptsLeft = 0
	}

	result := &LoginAttemptResult{
		Allowed:      count < maxLoginAttempts,
		AttemptsLeft: attemptsLeft,
	}

	if count >= maxLoginAttempts {
		lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
		lockedUntil := time.Now().Add(loginLockoutDuration)
		r.Client.Set(ctx, lockoutKey, lockedUntil.Unix(), loginLockoutDuration)
		result.LockedUntil = &lockedUntil
		result.LockoutDuration = loginLockoutDuration
	}

	return result, nil
}

// IsLoginLocked reports whether the identifier is currently locked out, and
// until when. Returns (false, nil, nil) when not locked.
func (r *RedisClient) IsLoginLocked(ctx context.Context, identifier string) (bool, *time.Time, error) {
	lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
	timestamp, err := r.Client.Get(ctx, lockoutKey).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil, nil
		}
		return false, nil, err
	}

	lockedUntil := time.Unix(timestamp, 0)
	if time.Now().After(lockedUntil) {
		r.Client.Del(ctx, lockoutKey)
		return false, nil, nil
	}

	return true, &lockedUntil, nil
}

// ClearLoginAttempts resets the attempt counter and lockout for an identifier.
func (r *RedisClient) ClearLoginAttempts(ctx context.Context, identifier string) error {
	key := r.Key(loginAttemptPrefix, identifier)
	lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
	return r.Client.Del(ctx, key, lockoutKey).Err()
}

// =============================================================================
// Idempotency Store — prevents duplicate offline mutations (PWA sync)
// =============================================================================

// IdempotencyStore is the interface consumed by the idempotency middleware.
// The key is the client-supplied Idempotency-Key header; userID scopes it so
// different operators cannot collide.
type IdempotencyStore interface {
	Get(key, userID string) ([]byte, bool)
	Set(key, userID string, data []byte, ttl time.Duration)
}

const idempotencyPrefix = "idempotency"

// RedisIdempotencyStore implements IdempotencyStore backed by Redis.
// Each entry is keyed as "cpx:idempotency:{userID}:{key}" and expires after
// the requested TTL (architecture specifies 24 hours).
type RedisIdempotencyStore struct {
	rc *RedisClient
}

// NewIdempotencyStore creates a Redis-backed IdempotencyStore.
func NewIdempotencyStore(rc *RedisClient) *RedisIdempotencyStore {
	return &RedisIdempotencyStore{rc: rc}
}

// Get retrieves a cached idempotent response. Returns (nil, false) on miss.
func (s *RedisIdempotencyStore) Get(key, userID string) ([]byte, bool) {
	ctx := context.Background()
	redisKey := s.rc.Key(idempotencyPrefix, userID, key)
	data, err := s.rc.Client.Get(ctx, redisKey).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

// Set caches an idempotent response body with the given TTL.
func (s *RedisIdempotencyStore) Set(key, userID string, data []byte, ttl time.Duration) {
	ctx := context.Background()
	redisKey := s.rc.Key(idempotencyPrefix, userID, key)
	s.rc.Client.Set(ctx, redisKey, data, ttl) // best-effort cache; Set returns *StatusCmd not error
}

// =============================================================================
// Redis Streams — fl:generate (MVP: only stream)
// =============================================================================

// streamKey returns the namespaced Redis stream key for AI draft generation.
// Using r.Key() keeps it consistent with every other key in the package and
// prevents cross-contamination when multiple environments share a Redis instance.
// fl:deliver and fl:reply are deferred to Sprint G.
func (r *RedisClient) streamKey() string {
	return r.Key("fl", "generate")
}

// GeneratePayload is the message body written to fl:generate by the HTTP handler
// and read by the AI Draft Worker goroutine.
type GeneratePayload struct {
	InvoiceID  string `json:"invoice_id"`
	UserID     string `json:"user_id"`
	CustomerID string `json:"customer_id"`
	Tone       string `json:"tone"`    // polite | firm | urgent
	Channel    string `json:"channel"` // line | whatsapp | messenger | telegram | external
	AttemptNo  int    `json:"attempt_no"`
}

// EnqueueDraftGeneration adds a GeneratePayload to the fl:generate stream.
// Called by GET /api/v1/invoices/{id}/messages when no existing draft is found.
// Returns the Redis stream entry ID (used for tracing; the worker uses XREADGROUP).
func (r *RedisClient) EnqueueDraftGeneration(ctx context.Context, payload GeneratePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal generate payload: %w", err)
	}

	id, err := r.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamKey(),
		ID:     "*", // auto-assign
		Values: map[string]any{
			"payload": string(data),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd fl:generate failed: %w", err)
	}
	return id, nil
}

// EnsureConsumerGroup creates the consumer group on fl:generate if it does not
// already exist. Call once during worker startup. The group reads from ">" (new
// messages only). MKSTREAM creates the stream if it is empty.
func (r *RedisClient) EnsureConsumerGroup(ctx context.Context, group string) error {
	err := r.Client.XGroupCreateMkStream(ctx, r.streamKey(), group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("failed to create consumer group %q: %w", group, err)
	}
	return nil
}

// ReadGenerateStream reads up to count pending messages from fl:generate for a
// given consumer in the group. Set blockDuration > 0 to block until a message
// arrives (e.g. 5 * time.Second). Pass ">" as lastID to read new messages, or
// "0" to re-deliver pending (unacked) messages after a crash.
func (r *RedisClient) ReadGenerateStream(ctx context.Context, group, consumer string, count int64, block time.Duration, lastID string) ([]GeneratePayload, []string, error) {
	streams, err := r.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{r.streamKey(), lastID},
		Count:    count,
		Block:    block,
		NoAck:    false,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil, nil // Timeout with no messages — normal
		}
		return nil, nil, fmt.Errorf("xreadgroup failed: %w", err)
	}

	var payloads []GeneratePayload
	var entryIDs []string

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			raw, ok := msg.Values["payload"].(string)
			if !ok {
				continue
			}
			var p GeneratePayload
			if err := json.Unmarshal([]byte(raw), &p); err != nil {
				continue
			}
			payloads = append(payloads, p)
			entryIDs = append(entryIDs, msg.ID)
		}
	}

	return payloads, entryIDs, nil
}

// AckGenerateStream acknowledges one or more stream entry IDs so Redis removes
// them from the pending-entries list. Call after the worker successfully writes
// the draft message to the database.
func (r *RedisClient) AckGenerateStream(ctx context.Context, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.Client.XAck(ctx, r.streamKey(), group, ids...).Err()
}

// =============================================================================
// Pub/Sub — SSE draft_ready notifications
// =============================================================================

// DraftReadyEvent is published on the user-scoped SSE channel after the AI
// Draft Worker writes the message row. The HTTP SSE handler subscribes and
// forwards this event to the waiting client.
type DraftReadyEvent struct {
	Event     string `json:"event"`      // always "draft_ready"
	MessageID string `json:"message_id"` // UUID of the new messages row
	InvoiceID string `json:"invoice_id"`
}

const draftReadyChannelPrefix = "sse:draft_ready"

// PublishDraftReady notifies the SSE handler that an AI draft is ready.
// Called by the AI Draft Worker after successfully writing the messages row.
func (r *RedisClient) PublishDraftReady(ctx context.Context, userID, messageID, invoiceID string) error {
	event := DraftReadyEvent{
		Event:     "draft_ready",
		MessageID: messageID,
		InvoiceID: invoiceID,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	channel := r.Key(draftReadyChannelPrefix, userID)
	return r.Client.Publish(ctx, channel, data).Err()
}

// SubscribeDraftReady returns a Pub/Sub subscription for the user's draft_ready
// channel. The SSE handler calls this once per connection and forwards each
// received message as an SSE event to the browser.
func (r *RedisClient) SubscribeDraftReady(ctx context.Context, userID string) *redis.PubSub {
	channel := r.Key(draftReadyChannelPrefix, userID)
	return r.Client.Subscribe(ctx, channel)
}

// Publish publishes an arbitrary JSON message to a Redis channel.
func (r *RedisClient) Publish(ctx context.Context, channel string, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return r.Client.Publish(ctx, r.Key(channel), data).Err()
}

// Subscribe returns a subscription to one or more channels (prefixed).
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	prefixedChannels := make([]string, len(channels))
	for i, ch := range channels {
		prefixedChannels[i] = r.Key(ch)
	}
	return r.Client.Subscribe(ctx, prefixedChannels...)
}

// =============================================================================
// PIN Reset OTP
// =============================================================================

// StoreOTP persists a 6-digit OTP for the given userID with a 10-minute TTL.
// Calling this again before expiry overwrites the previous OTP.
func (r *RedisClient) StoreOTP(ctx context.Context, userID, otp string) error {
	key := r.Key(otpResetPrefix, userID)
	return r.Client.Set(ctx, key, otp, otpResetTTL).Err()
}

// GetOTP retrieves the OTP stored for a userID. Returns ErrTokenNotFound when absent or expired.
func (r *RedisClient) GetOTP(ctx context.Context, userID string) (string, error) {
	key := r.Key(otpResetPrefix, userID)
	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrTokenNotFound
		}
		return "", err
	}
	return val, nil
}

// DeleteOTP removes the OTP for a userID after it has been consumed.
func (r *RedisClient) DeleteOTP(ctx context.Context, userID string) error {
	key := r.Key(otpResetPrefix, userID)
	return r.Client.Del(ctx, key).Err()
}

// =============================================================================
// Registration Email OTP (keyed by email, pre-account-creation)
// =============================================================================

// StoreRegOTP persists a 6-digit registration OTP keyed by email with a 10-minute TTL.
func (r *RedisClient) StoreRegOTP(ctx context.Context, email, otp string) error {
	key := r.Key(otpRegPrefix, email)
	return r.Client.Set(ctx, key, otp, otpRegTTL).Err()
}

// GetRegOTP retrieves the registration OTP for the given email. Returns ErrTokenNotFound when absent or expired.
func (r *RedisClient) GetRegOTP(ctx context.Context, email string) (string, error) {
	key := r.Key(otpRegPrefix, email)
	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrTokenNotFound
		}
		return "", err
	}
	return val, nil
}

// DeleteRegOTP removes the registration OTP for an email after it has been consumed.
func (r *RedisClient) DeleteRegOTP(ctx context.Context, email string) error {
	key := r.Key(otpRegPrefix, email)
	return r.Client.Del(ctx, key).Err()
}
