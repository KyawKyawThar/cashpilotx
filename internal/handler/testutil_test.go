package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/config"
	"cashpilotx/internal/server"
	"cashpilotx/internal/token"
	"cashpilotx/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

// testEnv holds the shared server, store, and token maker for HTTP scenario tests.
type testEnv struct {
	baseURL    string
	store      db.Store
	pool       *pgxpool.Pool
	tokenMaker token.Maker
	httpServer *http.Server
}

// newTestEnv spins up a real server bound to a free port. t.Cleanup shuts it down.
// Mirrors the pattern in sse_test.go.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping HTTP integration test in short mode")
	}

	cfg, err := config.LoadConfig("../../")
	require.NoError(t, err)
	cfg.Environment = "development"
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = "0"

	// Isolated test DB — never the dev/prod database (see TestDBSourceOrDerive).
	testDBSource, err := cfg.TestDBSourceOrDerive()
	require.NoError(t, err)

	pool, err := pgxpool.New(context.Background(), testDBSource)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	store := db.NewStore(pool)

	redisClient, err := cache.NewRedisClient(cache.RedisConfig{Address: cfg.RedisURL})
	require.NoError(t, err)

	srv, err := server.NewServer(store, redisClient, cfg)
	require.NoError(t, err)

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp",
		fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort))
	require.NoError(t, err)

	httpSrv := &http.Server{Handler: srv.Router()}
	go func() {
		if serveErr := httpSrv.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error().Err(serveErr).Msg("test server error")
		}
	}()
	t.Cleanup(func() {
		require.NoError(t, httpSrv.Shutdown(context.Background()))
	})

	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret, cfg.JWTIssuer)
	require.NoError(t, err)

	return &testEnv{
		baseURL:    "http://" + listener.Addr().String(),
		store:      store,
		pool:       pool,
		tokenMaker: tokenMaker,
		httpServer: httpSrv,
	}
}

// seedUser creates a committed user directly in the DB and returns the user and
// its access token. Bypasses the OTP email flow — suitable for integration tests only.
func (e *testEnv) seedUser(t *testing.T) (user db.User, tok string) {
	t.Helper()

	phone := utils.RandomPhone(9)
	bn := utils.RandomString(8)
	locale := "en"

	pinHash, err := utils.HashPassword("Test1234!")
	require.NoError(t, err)

	user, err = e.store.CreateUser(context.Background(), db.CreateUserParams{
		Phone:        &phone,
		PinHash:      pinHash,
		Name:         utils.RandomString(6),
		BusinessName: &bn,
		Locale:       &locale,
		Plan:         "pro",
		HomeCurrency: "THB",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	tok, _, err = e.tokenMaker.CreateToken(token.CreateTokenParams{
		UserID:   user.ID,
		Duration: time.Hour,
		Plan:     "pro",
		Role:     string(user.Role),
	})
	require.NoError(t, err)

	return user, tok
}

// mustDo performs an HTTP request, asserts wantStatus, optionally decodes the
// standard `data` envelope into dst, and always closes the response body.
// Pass nil for dst when only the status code matters.
func (e *testEnv) mustDo(t *testing.T, method, path string, body any, authToken string, wantStatus int, dst any) {
	t.Helper()

	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, e.baseURL+path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		_, _ = io.Copy(io.Discard, resp.Body)
		t.Fatalf("expected HTTP %d, got %d — %s %s", wantStatus, resp.StatusCode, method, path)
		return
	}

	if dst != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
		require.NoError(t, json.Unmarshal(envelope.Data, dst))
	}
}

// mustDoIdempotent is like mustDo but adds an Idempotency-Key header.
// Required for POST /payments and PATCH /invoices/{id}/collection-state.
func (e *testEnv) mustDoIdempotent(t *testing.T, method, path string, body any, authToken string, wantStatus int, dst any) {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), method, e.baseURL+path, bytes.NewBuffer(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Idempotency-Key", uuid.New().String())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		_, _ = io.Copy(io.Discard, resp.Body)
		t.Fatalf("expected HTTP %d, got %d — %s %s", wantStatus, resp.StatusCode, method, path)
		return
	}

	if dst != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
		require.NoError(t, json.Unmarshal(envelope.Data, dst))
	}
}

// mustGetStatus performs a request and returns only the status code, closing the body.
func (e *testEnv) mustGetStatus(t *testing.T, method, path, authToken string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, e.baseURL+path, http.NoBody)
	require.NoError(t, err)
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
