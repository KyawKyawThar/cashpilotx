// Package main is the entry point for the background worker.
package main

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/config"
	"cashpilotx/internal/email"
	"cashpilotx/internal/notify"
	"cashpilotx/internal/worker"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Error().Err(err).Msg("failed to load config")
		os.Exit(1)
	}
	setupLogger(cfg.Environment)

	if err := runWorker(cfg); err != nil {
		log.Error().Err(err).Msg("worker exited with error")
		os.Exit(1)
	}
}

func runWorker(cfg config.Config) error {
	log.Info().Str("environment", cfg.Environment).Msg("starting draft worker")

	connPool, err := setupDatabase(cfg.DBSource)
	if err != nil {
		return fmt.Errorf("setup database: %w", err)
	}
	defer connPool.Close()

	redisClient, err := setupRedis(&cfg)
	if err != nil {
		return fmt.Errorf("setup redis: %w", err)
	}
	if redisClient == nil {
		return fmt.Errorf("REDIS_URL is required for the draft worker")
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("error closing Redis client")
		}
	}()

	hostname, _ := os.Hostname() //nolint:errcheck // empty hostname is acceptable
	consumer := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	store := db.NewStore(connPool)
	w := worker.New(store, redisClient, consumer)

	// Build email sender (same logic as API server: Resend in prod, SMTP elsewhere).
	var emailSender email.Sender
	if cfg.Environment == config.EnvironmentProduction {
		emailSender = email.NewResendSender(cfg.SMTPPassword, cfg.EmailFrom)
	} else {
		emailSender = email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.EmailFrom, cfg.SMTPUsername, cfg.SMTPPassword)
	}

	// Build push sender — optional; only active when VAPID keys are configured.
	var pushSender *notify.Sender
	if cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "" {
		pushSender = notify.NewSender(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDEmail)
	}

	digest := worker.NewDigestScheduler(store, emailSender, pushSender, cfg.DigestHourUTC, cfg.AppURL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	workerDone := make(chan error, 1)
	go func() {
		workerDone <- w.Run(ctx)
	}()
	go digest.Run(ctx)

	select {
	case sig := <-shutdown:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
		select {
		case <-workerDone:
		case <-time.After(15 * time.Second):
			log.Warn().Msg("worker did not stop within 15 s, forcing exit")
		}
	case runErr := <-workerDone:
		if runErr != nil {
			return runErr
		}
	}

	log.Info().Msg("draft worker stopped")
	return nil
}

func setupLogger(environment string) {
	if environment == config.EnvironmentDevelopment || environment == "dev" {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
			With().Timestamp().Logger()
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Str("service", "cashpilotx-worker").Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func setupDatabase(dbSource string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(dbSource)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	poolConfig.MaxConns = 4
	poolConfig.MinConns = 1
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 10 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func setupRedis(cfg *config.Config) (*cache.RedisClient, error) {
	if cfg.RedisURL == "" {
		return nil, nil //nolint:nilnil // caller validates nil and returns an error
	}
	rc := cache.RedisConfig{Address: cfg.RedisURL}
	if cfg.RedisPassword != "" {
		rc.Password = cfg.RedisPassword
	}
	if cfg.RedisDB != 0 {
		rc.DB = cfg.RedisDB
	}
	return cache.NewRedisClient(rc)
}
