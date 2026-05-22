package handler_test

import (
	"bufio"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/config"
	"cashpilotx/internal/server"
	"cashpilotx/internal/token"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	db "cashpilotx/db/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

func TestSSEHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// 1. Load config
	cfg, err := config.LoadConfig("../../")
	require.NoError(t, err)
	cfg.Environment = "development"
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = "0"

	// 2. Connect real DB and Redis (integration test)
	pool, err := pgxpool.New(context.Background(), cfg.DBSource)
	require.NoError(t, err)
	defer pool.Close()

	store := db.NewStore(pool)

	redisClient, err := cache.NewRedisClient(cache.RedisConfig{Address: cfg.RedisURL})
	require.NoError(t, err)

	// 3. Build server (wires handlers + SSE internally)
	srv, err := server.NewServer(store, redisClient, cfg)
	require.NoError(t, err)

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort))
	require.NoError(t, err)

	httpServer := &http.Server{Handler: srv.Router()}
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error().Err(serveErr).Msg("test server error")
		}
	}()
	defer func() {
		require.NoError(t, httpServer.Shutdown(context.Background()))
	}()

	// 4. Create a token for the test user
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret, cfg.JWTIssuer)
	require.NoError(t, err)

	userID := uuid.MustParse("bf1f54dc-3b44-4d7a-8f01-0530e05020bb")
	accessToken, _, err := tokenMaker.CreateToken(token.CreateTokenParams{UserID: userID, Duration: time.Hour, Plan: "free"})
	require.NoError(t, err)

	// 5. Open SSE stream
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("http://%s/api/v1/messages/sse", listener.Addr().String()), http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// 6. Read connection confirmation comment
	scanner := bufio.NewScanner(resp.Body)
	require.True(t, scanner.Scan())
	require.Equal(t, fmt.Sprintf(": connected user=%s", userID), scanner.Text())

	// 7. Publish a draft_ready event from Redis in the background
	go func() {
		time.Sleep(100 * time.Millisecond)
		err := redisClient.PublishDraftReady(context.Background(), userID.String(), "test-message-id", "test-invoice-id")
		require.NoError(t, err)
	}()

	// 8. SSE protocol: blank line separates the comment from the event
	require.True(t, scanner.Scan())
	require.Equal(t, "", scanner.Text())

	// 9. Read the event lines
	require.True(t, scanner.Scan())
	require.Equal(t, "event: draft_ready", scanner.Text())

	require.True(t, scanner.Scan())
	require.Contains(t, scanner.Text(), "test-message-id")
}
