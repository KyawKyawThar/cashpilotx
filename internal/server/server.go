package server

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/config"
	"cashpilotx/internal/email"
	"cashpilotx/internal/handler"
	"cashpilotx/internal/service"
	"cashpilotx/internal/token"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type Server struct {
	store      db.Store
	cache      *cache.RedisClient
	config     *config.Config
	router     *chi.Mux
	logger     *zerolog.Logger
	handler    *handler.Handlers
	services   *service.Services
	tokenMaker token.Maker
}

func NewServer(store db.Store, redisCache *cache.RedisClient, cfg config.Config) (*Server, error) {
	issuer := cfg.JWTIssuer
	if issuer == "" {
		issuer = "cashpilotx"
	}

	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret, issuer)
	if err != nil {
		return nil, fmt.Errorf("cannot create token maker: %w", err)
	}

	server := &Server{
		store:      store,
		cache:      redisCache,
		config:     &cfg,
		tokenMaker: tokenMaker,
	}

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if server.config.Environment == config.EnvironmentDevelopment {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	}
	server.logger = &logger

	// Fly.io blocks outbound SMTP port 587. Use Resend's HTTP API in production.
	var emailSender email.Sender
	if cfg.Environment == config.EnvironmentProduction {
		emailSender = email.NewResendSender(cfg.SMTPPassword, cfg.EmailFrom)
	} else {
		emailSender = email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.EmailFrom, cfg.SMTPUsername, cfg.SMTPPassword)
	}

	encKey, err := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid ENCRYPTION_KEY (must be base64-encoded 32-byte key): %w", err)
	}

	services := service.NewServices(server.store, server.cache, server.tokenMaker, &service.Config{
		Auth: service.AuthConfig{
			AccessTokenDuration:  cfg.AccessTokenDuration,
			RefreshTokenDuration: cfg.RefreshTokenDuration,
			BcryptCost:           10,
			JWTIssuer:            issuer,
			Environment:          cfg.Environment,
		},
		Customer: service.CustomerConfig{
			ChannelEncryptionKey: encKey,
		},
	}, emailSender)
	server.services = services

	deps := &handler.HandlerDeps{}
	if redisCache != nil {
		deps.RedisClient = redisCache.Client
		deps.RedisCache = redisCache
		deps.IngestStreamName = cfg.RedisStreamGenerate
	}

	server.handler = handler.NewHandlers(services, server.store, server.logger, deps)
	server.setupRoutes()
	return server, nil
}

func (s *Server) Start(address string) error {
	srv := &http.Server{
		Addr:         address,
		Handler:      s.router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{
				tls.X25519,
				tls.CurveP256,
			},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
	}
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	})
	return srv.ListenAndServeTLS("cert.pem", "key.pem")
}

func (s *Server) Router() http.Handler {
	return s.router
}
