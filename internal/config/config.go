// Package config loads and validates application configuration from environment variables.
package config

import (
	"cashpilotx/utils"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvironmentDevelopment is a constant for the development environment.
const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"
)

// StatusHealthy is a constant for the healthy status.
const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

// Config is the configuration for the application.
type Config struct {
	// ── App ──────────────────────────────────────────────────────
	Environment string `mapstructure:"APP_ENV"`
	AppVersion  string `mapstructure:"APP_VERSION"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`
	LogFormat   string `mapstructure:"LOG_FORMAT"`

	// ── Server ───────────────────────────────────────────────────
	ServerPort         string        `mapstructure:"SERVER_PORT"`
	ServerHost         string        `mapstructure:"SERVER_HOST"`
	ServerReadTimeout  time.Duration `mapstructure:"SERVER_READ_TIMEOUT"`
	ServerWriteTimeout time.Duration `mapstructure:"SERVER_WRITE_TIMEOUT"`
	ServerIdleTimeout  time.Duration `mapstructure:"SERVER_IDLE_TIMEOUT"`

	// ── PostgreSQL ───────────────────────────────────────────────
	DBSource          string `mapstructure:"DATABASE_URL"`
	DBPoolMaxConns    int    `mapstructure:"DB_POOL_MAX_CONNS"`
	DBPoolMinConns    int    `mapstructure:"DB_POOL_MIN_CONNS"`
	DBPoolMaxConnLife string `mapstructure:"DB_POOL_MAX_CONN_LIFETIME"`
	DBPoolMaxConnIdle string `mapstructure:"DB_POOL_MAX_CONN_IDLE_TIME"`

	// ── Redis ────────────────────────────────────────────────────
	RedisURL            string `mapstructure:"REDIS_URL"`
	RedisPassword       string `mapstructure:"REDIS_PASSWORD"`
	RedisDB             int    `mapstructure:"REDIS_DB"`
	RedisMaxMemory      string `mapstructure:"REDIS_MAXMEMORY"`
	RedisStreamGenerate string `mapstructure:"REDIS_STREAM_GENERATE"`
	RedisStreamDeliver  string `mapstructure:"REDIS_STREAM_DELIVER"`
	RedisStreamReply    string `mapstructure:"REDIS_STREAM_REPLY"`

	// ── S3 / MinIO ───────────────────────────────────────────────
	S3Endpoint       string `mapstructure:"S3_ENDPOINT"`
	S3Bucket         string `mapstructure:"S3_BUCKET"`
	S3Region         string `mapstructure:"S3_REGION"`
	S3AccessKey      string `mapstructure:"S3_ACCESS_KEY"`
	S3SecretKey      string `mapstructure:"S3_SECRET_KEY"`
	S3ForcePathStyle bool   `mapstructure:"S3_FORCE_PATH_STYLE"`

	// ── SMTP ─────────────────────────────────────────────────────
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUsername string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	EmailFrom    string `mapstructure:"EMAIL_FROM"`
	ClientAppURL string `mapstructure:"CLIENT_APP_URL"`

	// ── JWT Auth ─────────────────────────────────────────────────
	JWTSecret            string        `mapstructure:"JWT_SECRET"`
	AccessTokenDuration  time.Duration `mapstructure:"JWT_ACCESS_TOKEN_TTL"`
	RefreshTokenDuration time.Duration `mapstructure:"JWT_REFRESH_TOKEN_TTL"`
	JWTIssuer            string        `mapstructure:"JWT_ISSUER"`

	// ── Encryption ───────────────────────────────────────────────
	EncryptionKey string `mapstructure:"ENCRYPTION_KEY"`

	// ── API Keys ─────────────────────────────────────────────────
	APIKeyPrefix     string `mapstructure:"API_KEY_PREFIX"`
	APIKeyHashPepper string `mapstructure:"API_KEY_HASH_PEPPER"`

	// ── Workers ──────────────────────────────────────────────────
	WorkerReplicas    int `mapstructure:"WORKER_REPLICAS"`
	WorkerConcurrency int `mapstructure:"WORKER_CONCURRENCY"`
	WorkerBatchSize   int `mapstructure:"WORKER_BATCH_SIZE"`
	WorkerMaxRetries  int `mapstructure:"WORKER_MAX_RETRIES"`

	// ── Rate Limiting ────────────────────────────────────────────
	RateLimitBasic    int `mapstructure:"RATE_LIMIT_BASIC"`
	RateLimitPro      int `mapstructure:"RATE_LIMIT_PRO"`
	RateLimitBusiness int `mapstructure:"RATE_LIMIT_BUSINESS"`

	// ── Feature Flags ────────────────────────────────────────────
	FeatureTradeModule      bool `mapstructure:"FEATURE_TRADE_MODULE"`
	FeatureTeams            bool `mapstructure:"FEATURE_TEAMS"`
	FeatureWeChat           bool `mapstructure:"FEATURE_WECHAT"`
	FeatureRiskIntelligence bool `mapstructure:"FEATURE_RISK_INTELLIGENCE"`

	// ── Stripe ───────────────────────────────────────────────────
	StripeSecretKey     string `mapstructure:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret string `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	StripePriceBasic    string `mapstructure:"STRIPE_PRICE_BASIC"`
	StripePricePro      string `mapstructure:"STRIPE_PRICE_PRO"`
	StripePriceBusiness string `mapstructure:"STRIPE_PRICE_BUSINESS"`

	// ── CORS ─────────────────────────────────────────────────────
	CORSAllowedOrigins   string   `mapstructure:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool     `mapstructure:"CORS_ALLOW_CREDENTIALS"`
	AllowedOrigins       []string // parsed from CORSAllowedOrigins
}

// LoadConfig loads the configuration from the given path.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	mustBindEnv(
		"APP_ENV", "DATABASE_URL", "REDIS_URL",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD",
		"EMAIL_FROM", "CLIENT_APP_URL",
		"S3_BUCKET", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_REGION",
		"JWT_SECRET", "JWT_ACCESS_TOKEN_TTL", "JWT_REFRESH_TOKEN_TTL",
		"CORS_ALLOWED_ORIGINS",
	)

	viper.SetDefault("APP_ENV", EnvironmentDevelopment)
	viper.SetDefault("SERVER_HOST", "0.0.0.0")
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("JWT_ACCESS_TOKEN_TTL", "15m")
	viper.SetDefault("JWT_REFRESH_TOKEN_TTL", "168h")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("JWT_SECRET", utils.RandomString(32))
	viper.SetDefault("DATABASE_URL", "")
	viper.SetDefault("REDIS_URL", "")
	viper.SetDefault("S3_BUCKET", "")
	viper.SetDefault("S3_ENDPOINT", "")
	viper.SetDefault("S3_ACCESS_KEY", "")
	viper.SetDefault("S3_SECRET_KEY", "")
	viper.SetDefault("S3_REGION", "")
	viper.SetDefault("RATE_LIMIT_BASIC", 60)
	viper.SetDefault("RATE_LIMIT_PRO", 180)
	viper.SetDefault("RATE_LIMIT_BUSINESS", 600)
	viper.SetDefault("WORKER_REPLICAS", 2)
	viper.SetDefault("WORKER_CONCURRENCY", 4)
	viper.SetDefault("WORKER_BATCH_SIZE", 50)
	viper.SetDefault("WORKER_MAX_RETRIES", 3)

	err = viper.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return config, err
		}
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return config, err
	}

	// Trim string fields used for equality comparisons — Makefile `include .env`
	// can add trailing whitespace when lines have inline comments.
	config.Environment = strings.TrimSpace(config.Environment)
	config.LogLevel = strings.TrimSpace(config.LogLevel)
	config.LogFormat = strings.TrimSpace(config.LogFormat)

	// Parse CORS_ALLOWED_ORIGINS into AllowedOrigins slice.
	if config.CORSAllowedOrigins != "" {
		config.AllowedOrigins = parseOrigins(config.CORSAllowedOrigins)
	}

	return config, nil
}

// mustBindEnv binds each key to its matching OS environment variable.
func mustBindEnv(keys ...string) {
	for _, k := range keys {
		if err := viper.BindEnv(k); err != nil {
			panic(fmt.Sprintf("config: BindEnv(%q): %v", k, err))
		}
	}
}

func parseOrigins(s string) []string {
	if s == "*" {
		return []string{"*"}
	}
	origins := strings.Split(s, ",")
	result := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			result = append(result, origin)
		}
	}
	return result
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvironmentDevelopment
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}
