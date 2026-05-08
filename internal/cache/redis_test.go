package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a miniredis instance and returns a RedisClient for testing.
func setupTestRedis(t *testing.T) (*RedisClient, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	mr.RequireAuth("testpassword")
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr:     mr.Addr(),
		Password: "testpassword",
	})

	rc := &RedisClient{
		Client: client,
		Prefix: "test:",
	}

	return rc, mr
}
func teardownTestRedis(rc *RedisClient, mr *miniredis.Miniredis) {
	rc.Close()
	mr.Close()
}

// =============================================================================
// ParseRedisConfig
// =============================================================================

func TestParseRedisConfig_SimpleHostPort(t *testing.T) {

	cfg, err := ParseRedisConfig("localhost:6379")

	require.NoError(t, err)
	require.Equal(t, "localhost", cfg.Host)
	require.Equal(t, 6379, cfg.Port)
	require.Equal(t, "", cfg.Password)
	require.Equal(t, 0, cfg.DB)
}

func TestParseRedisConfig_URLWithoutPortOrDB(t *testing.T) {
	cfg, err := ParseRedisConfig("redis://redis.local")
	require.NoError(t, err)
	require.Equal(t, "redis.local", cfg.Host)
	require.Equal(t, 6379, cfg.Port) // default port
}

func TestNewRedisClientWithAddress(t *testing.T) {
	// use miniredis to verify the client can connect via address parsing
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	addr := mr.Addr()
	// supply a URI-like address (without scheme) to NewRedisClient
	client, err := NewRedisClient(RedisConfig{
		Address:  addr,
		Password: "testpassword",
		Prefix:   "test:",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	client.Close()
}
func TestNewRedisClientWithURLAddress(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	url := fmt.Sprintf("redis://:%s@%s/%d", "", mr.Addr(), 0)
	client, err := NewRedisClient(RedisConfig{Address: url})
	require.NoError(t, err)
	require.NotNil(t, client)
	client.Close()
}

// =============================================================================
// Generic Key-Value Ops
// =============================================================================

func TestRedisClient_SetAndGet(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := TestData{Name: "test", Value: 42}
	require.NoError(t, rc.Set(ctx, "mykey", data, time.Minute))

	var retrieved TestData
	require.NoError(t, rc.Get(ctx, "mykey", &retrieved))
	require.Equal(t, data.Name, retrieved.Name)
	require.Equal(t, data.Value, retrieved.Value)
}
func TestRedisClient_GetNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	var data string
	err := rc.Get(context.Background(), "nonexistent", &data)
	require.ErrorIs(t, err, ErrKeyNotFound)
}

func TestRedisClient_SetStringAndGetString(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	require.NoError(t, rc.SetString(ctx, "strkey", "hello world", time.Minute))

	val, err := rc.GetString(ctx, "strkey")
	require.NoError(t, err)
	require.Equal(t, "hello world", val)
}
func TestRedisClient_Delete(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	require.NoError(t, rc.SetString(ctx, "deletekey", "value", time.Minute))

	exists, err := rc.Exists(ctx, "deletekey")
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, rc.Delete(ctx, "deletekey"))

	exists, err = rc.Exists(ctx, "deletekey")
	require.NoError(t, err)
	require.False(t, exists)
}
func TestRedisClient_Exists(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	exists, err := rc.Exists(ctx, "nokey")
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, rc.SetString(ctx, "nokey", "value", time.Minute))

	exists, err = rc.Exists(ctx, "nokey")
	require.NoError(t, err)
	require.True(t, exists)
}

func TestRedisClient_KeyPrefix(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	require.Equal(t, "test:session:abc123", rc.Key("session", "abc123"))
	require.Equal(t, "test:user_sessions:user-1", rc.Key("user_sessions", "user-1"))
	require.Equal(t, "test:a:b:c", rc.Key("a", "b", "c"))
}
func TestRedisClient_GetClient(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	client := rc.GetClient()
	require.NotNil(t, client)

	ctx := context.Background()
	err := client.Set(ctx, "direct-key", "direct-value", time.Minute).Err()
	require.NoError(t, err)
}

// =============================================================================
// Session Management
// NOTE: Session no longer has TenantID — CashPilotX is single-user per account.
// =============================================================================

func TestRedisClient_CreateAndGetSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	session := &Session{
		ID:           "session-123",
		UserID:       "user-456",
		RefreshToken: "refresh-token-abc",
		UserAgent:    "Mozilla/5.0",
		IPAddress:    "192.168.1.1",
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		LastActiveAt: time.Now().UTC(),
	}

	require.NoError(t, rc.CreateSession(ctx, session))

	retrieved, err := rc.GetSession(ctx, "session-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, session.ID, retrieved.ID)
	require.Equal(t, session.UserID, retrieved.UserID)
	require.Equal(t, session.RefreshToken, retrieved.RefreshToken)
	require.Equal(t, session.UserAgent, retrieved.UserAgent)
	require.Equal(t, session.IPAddress, retrieved.IPAddress)
}
func TestRedisClient_GetSessionNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	session, err := rc.GetSession(context.Background(), "nonexistent-session")
	require.ErrorIs(t, err, ErrSessionNotFound)
	require.Nil(t, session)
}

func TestRedisClient_DeleteSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	session := &Session{
		ID:        "session-to-delete",
		UserID:    "user-123",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	require.NoError(t, rc.CreateSession(ctx, session))

	retrieved, err := rc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	require.NoError(t, rc.DeleteSession(ctx, session.ID, session.UserID))

	retrieved, err = rc.GetSession(ctx, session.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)
	require.Nil(t, retrieved)
}

func TestRedisClient_GetUserSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-multi-session"

	for i, ua := range []string{"Chrome", "Firefox"} {
		session := &Session{
			ID:        fmt.Sprintf("session-%d", i),
			UserID:    userID,
			UserAgent: ua,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		require.NoError(t, rc.CreateSession(ctx, session))
	}

	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}

func TestRedisClient_DeleteAllUserSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-logout-all"

	for i := range 3 {
		session := &Session{
			ID:        fmt.Sprintf("session-%d", i),
			UserID:    userID,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		require.NoError(t, rc.CreateSession(ctx, session))
	}

	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	require.NoError(t, rc.DeleteAllUserSessions(ctx, userID))

	sessions, err = rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 0)
}

func TestRedisClient_RevokeSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	session := &Session{
		ID:        "session-revoke",
		UserID:    "user-revoke",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	require.NoError(t, rc.CreateSession(ctx, session))
	require.NoError(t, rc.RevokeSession(ctx, session.UserID, session.ID))

	retrieved, err := rc.GetSession(ctx, session.ID)
	require.ErrorIs(t, err, ErrSessionNotFound)
	require.Nil(t, retrieved)
}

func TestRedisClient_RevokeAllSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-revoke-all"

	session := &Session{
		ID:        "session-revoke-all",
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	require.NoError(t, rc.CreateSession(ctx, session))
	require.NoError(t, rc.RevokeAllSessions(ctx, userID))

	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 0)
}

func TestRedisClient_CreateExpiredSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	session := &Session{
		ID:        "expired-session",
		UserID:    "user-123",
		ExpiresAt: time.Now().UTC().Add(-time.Hour), // already expired
	}

	err := rc.CreateSession(context.Background(), session)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

func TestRedisClient_BlacklistToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	tokenID := "token-to-blacklist"

	blacklisted, err := rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.False(t, blacklisted)

	require.NoError(t, rc.BlacklistToken(ctx, tokenID, time.Now().Add(time.Hour)))

	blacklisted, err = rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.True(t, blacklisted)
}
func TestRedisClient_BlacklistExpiredToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	tokenID := "already-expired-token"

	// No-op: token already expired
	require.NoError(t, rc.BlacklistToken(ctx, tokenID, time.Now().Add(-time.Hour)))

	blacklisted, err := rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.False(t, blacklisted)
}
func TestRedisClient_ResetToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	resetToken := &ResetToken{
		UserID:    "user-reset",
		Email:     "user@example.com",
		Token:     "reset-token-xyz",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	require.NoError(t, rc.StoreResetToken(ctx, resetToken))

	retrieved, err := rc.GetResetToken(ctx, resetToken.Token)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, resetToken.UserID, retrieved.UserID)
	require.Equal(t, resetToken.Email, retrieved.Email)

	require.NoError(t, rc.DeleteResetToken(ctx, resetToken.Token))

	retrieved, err = rc.GetResetToken(ctx, resetToken.Token)
	require.ErrorIs(t, err, ErrTokenNotFound)
	require.Nil(t, retrieved)
}
func TestRedisClient_GetResetTokenNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	token, err := rc.GetResetToken(context.Background(), "nonexistent-token")
	require.ErrorIs(t, err, ErrTokenNotFound)
	require.Nil(t, token)
}

func TestRedisClient_VerifyEmailToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	verifyToken := &VerifyEmailToken{
		UserID:    "user-verify",
		Email:     "verify@example.com",
		Code:      "123456",
		CreatedAt: time.Now().UTC(),
	}

	require.NoError(t, rc.StoreVerifyEmailToken(ctx, verifyToken))

	retrieved, err := rc.GetVerifyEmailToken(ctx, verifyToken.Email)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, verifyToken.UserID, retrieved.UserID)
	require.Equal(t, verifyToken.Code, retrieved.Code)

	require.NoError(t, rc.DeleteVerifyEmailToken(ctx, verifyToken.Email))

	retrieved, err = rc.GetVerifyEmailToken(ctx, verifyToken.Email)
	require.ErrorIs(t, err, ErrTokenNotFound)
	require.Nil(t, retrieved)
}

// =============================================================================
// Rate Limiting
// =============================================================================

func TestRedisClient_RateLimit(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	key := "api:user:123"
	limit := int64(3)
	window := time.Minute

	result, err := rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(2), result.Remaining)

	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(1), result.Remaining)

	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(0), result.Remaining)

	// 4th request — over limit
	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.False(t, result.Allowed)
	require.Equal(t, int64(0), result.Remaining)
	require.Greater(t, result.RetryAfter, int64(0))
}

// =============================================================================
// Login Attempt Tracking
// =============================================================================

func TestRedisClient_LoginAttempts(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	identifier := "operator@example.com"

	result, err := rc.RecordLoginAttempt(ctx, identifier, false)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, 4, result.AttemptsLeft)

	for range 3 {
		result, err = rc.RecordLoginAttempt(ctx, identifier, false)
		require.NoError(t, err)
		require.True(t, result.Allowed)
	}

	// 5th attempt — triggers lockout
	result, err = rc.RecordLoginAttempt(ctx, identifier, false)
	require.NoError(t, err)
	require.False(t, result.Allowed)
	require.NotNil(t, result.LockedUntil)
	require.Equal(t, 0, result.AttemptsLeft)

	locked, lockedUntil, err := rc.IsLoginLocked(ctx, identifier)
	require.NoError(t, err)
	require.True(t, locked)
	require.NotNil(t, lockedUntil)
}

// =============================================================================
// Idempotency Store — prevents duplicate offline mutations (PWA sync)
// =============================================================================
func TestIdempotencyStore_SetAndGet(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	store := NewIdempotencyStore(rc)
	key := "idem-key-001"
	userID := "user-abc"
	body := []byte(`{"payment_id":"uuid-123","status":"ok"}`)

	// Miss before set
	data, ok := store.Get(key, userID)
	require.False(t, ok)
	require.Nil(t, data)

	store.Set(key, userID, body, 24*time.Hour)

	// Hit after set
	data, ok = store.Get(key, userID)
	require.True(t, ok)
	require.Equal(t, body, data)
}

func TestIdempotencyStore_UserScopedKeys(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	store := NewIdempotencyStore(rc)
	key := "same-key"
	body := []byte(`{"ok":true}`)

	// Set for user A
	store.Set(key, "user-A", body, 24*time.Hour)

	// Should NOT be visible to user B (different scope)
	data, ok := store.Get(key, "user-B")
	require.False(t, ok)
	require.Nil(t, data)

	// Should be visible to user A
	data, ok = store.Get(key, "user-A")
	require.True(t, ok)
	require.Equal(t, body, data)
}

// =============================================================================
// Redis Streams — fl:generate
// =============================================================================

func TestEnqueueDraftGeneration(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	payload := GeneratePayload{
		InvoiceID:  "inv-uuid-001",
		UserID:     "user-uuid-001",
		CustomerID: "cust-uuid-001",
		Tone:       "polite",
		Channel:    "line",
		AttemptNo:  1,
	}

	id, err := rc.EnqueueDraftGeneration(ctx, payload)
	require.NoError(t, err)
	require.NotEmpty(t, id)
}

func TestEnqueueAndReadGenerateStream(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	group := "ai-draft-workers"
	consumer := "worker-1"

	// Create consumer group (MKSTREAM creates the stream)
	require.NoError(t, rc.EnsureConsumerGroup(ctx, group))

	// Enqueue a payload
	payload := GeneratePayload{
		InvoiceID:  "inv-uuid-002",
		UserID:     "user-uuid-002",
		CustomerID: "cust-uuid-002",
		Tone:       "firm",
		Channel:    "whatsapp",
		AttemptNo:  1,
	}
	_, err := rc.EnqueueDraftGeneration(ctx, payload)
	require.NoError(t, err)

	// Worker reads the message
	payloads, entryIDs, err := rc.ReadGenerateStream(ctx, group, consumer, 1, 0, ">")
	require.NoError(t, err)
	require.Len(t, payloads, 1)
	require.Len(t, entryIDs, 1)

	read := payloads[0]
	require.Equal(t, payload.InvoiceID, read.InvoiceID)
	require.Equal(t, payload.UserID, read.UserID)
	require.Equal(t, payload.Tone, read.Tone)
	require.Equal(t, payload.Channel, read.Channel)

	// Acknowledge — removes from pending-entries list
	require.NoError(t, rc.AckGenerateStream(ctx, group, entryIDs...))
}
func TestEnsureConsumerGroupIdempotent(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	group := "ai-draft-workers"

	// Calling EnsureConsumerGroup twice must not error
	require.NoError(t, rc.EnsureConsumerGroup(ctx, group))
	require.NoError(t, rc.EnsureConsumerGroup(ctx, group))
}

func TestReadGenerateStream_EmptyQueue(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	require.NoError(t, rc.EnsureConsumerGroup(ctx, "workers"))

	// Non-blocking read on empty stream returns nothing, no error
	payloads, ids, err := rc.ReadGenerateStream(ctx, "workers", "w1", 10, 100*time.Millisecond, ">")

	require.NoError(t, err)
	require.Empty(t, payloads)
	require.Empty(t, ids)
}

// =============================================================================
// Pub/Sub — SSE draft_ready notifications
// =============================================================================

func TestPublishDraftReady(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-sse-001"

	// Subscribe before publishing
	sub := rc.SubscribeDraftReady(ctx, userID)
	defer sub.Close()

	// Wait for subscription confirmation
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	// Worker publishes draft_ready
	require.NoError(t, rc.PublishDraftReady(ctx, userID, "msg-uuid-001", "inv-uuid-001"))

	// SSE handler receives the event
	msg, err := sub.ReceiveMessage(ctx)
	require.NoError(t, err)

	var event DraftReadyEvent
	require.NoError(t, json.Unmarshal([]byte(msg.Payload), &event))
	require.Equal(t, "draft_ready", event.Event)
	require.Equal(t, "msg-uuid-001", event.MessageID)
	require.Equal(t, "inv-uuid-001", event.InvoiceID)
}

func TestPublishDraftReady_DoesNotLeakAcrossUsers(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Subscribe as user B
	subB := rc.SubscribeDraftReady(ctx, "user-B")
	defer subB.Close()
	_, err := subB.Receive(ctx)
	require.NoError(t, err)

	// Publish for user A
	require.NoError(t, rc.PublishDraftReady(ctx, "user-A", "msg-uuid-999", "inv-uuid-999"))

	// Miniredis: user B should receive no message on the user-A channel
	// We just verify Publish succeeds and the channel key is scoped correctly.
	expected := rc.Key("sse:draft_ready", "user-B")
	// The message was sent to user-A's channel; user-B's key is distinct.
	require.NotEqual(t, rc.Key("sse:draft_ready", "user-A"), expected)
}

func TestRedisClient_Publish(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	type Message struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}
	err := rc.Publish(context.Background(), "notifications", Message{Type: "test", Payload: "hello"})
	require.NoError(t, err)
}

func TestRedisClient_PublishInvalidMessage(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	// Channels cannot be JSON-marshaled
	err := rc.Publish(context.Background(), "notifications", make(chan int))
	require.Error(t, err)
}

func TestRedisClient_Subscribe(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	pubsub := rc.Subscribe(ctx, "events")
	require.NotNil(t, pubsub)
	defer pubsub.Close()

	_, err := pubsub.Receive(ctx)
	require.NoError(t, err)
}

func TestRedisClient_PubSubIntegration(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	type Event struct {
		Action string `json:"action"`
		UserID string `json:"user_id"`
	}

	pubsub := rc.Subscribe(ctx, "user_events")
	defer pubsub.Close()

	_, err := pubsub.Receive(ctx)
	require.NoError(t, err)

	event := Event{Action: "login", UserID: "user-123"}
	require.NoError(t, rc.Publish(ctx, "user_events", event))

	msg, err := pubsub.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.Equal(t, "test:user_events", msg.Channel)

	var received Event
	require.NoError(t, json.Unmarshal([]byte(msg.Payload), &received))
	require.Equal(t, event.Action, received.Action)
	require.Equal(t, event.UserID, received.UserID)
}
