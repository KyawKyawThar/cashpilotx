package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/email"
	"cashpilotx/internal/token"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Services struct {
	Auth AuthServicer
}

type AuthService struct {
	store       db.Store
	cache       *cache.RedisClient
	tokenMaker  token.Maker
	config      *AuthConfig
	emailSender email.Sender
}

type AuthConfig struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	BcryptCost           int
	JWTIssuer            string
	Environment          string // "development" | "staging" | "production"
}

type Config struct {
	Auth AuthConfig
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		BcryptCost:           bcrypt.DefaultCost,
		JWTIssuer:            "cashpilotx",
		Environment:          "development",
	}
}

func DefaultConfig() *Config {
	return &Config{
		Auth: *DefaultAuthConfig(),
	}
}

func NewAuthService(store db.Store, redisCache *cache.RedisClient, tokenMaker token.Maker, cfg *AuthConfig, emailSender email.Sender) *AuthService {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}
	return &AuthService{
		store:       store,
		cache:       redisCache,
		tokenMaker:  tokenMaker,
		config:      cfg,
		emailSender: emailSender,
	}
}

func NewServices(store db.Store, redisCache *cache.RedisClient, tokenMaker token.Maker, cfg *Config, emailSender email.Sender) *Services {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	authSvc := NewAuthService(store, redisCache, tokenMaker, &cfg.Auth, emailSender)
	return &Services{
		Auth: authSvc,
	}
}
