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
	RedisPort           int    `mapstructure:"REDIS_PORT"`
	RedisHost           string `mapstructure:"REDIS_HOST"`
	RedisStreamGenerate string `mapstructure:"REDIS_STREAM_GENERATE"`

	// ── S3 / MinIO ───────────────────────────────────────────────
	S3Endpoint       string `mapstructure:"S3_ENDPOINT"`
	S3Bucket         string `mapstructure:"S3_BUCKET"`
	S3Region         string `mapstructure:"S3_REGION"`
	S3AccessKey      string `mapstructure:"S3_ACCESS_KEY"`
	S3SecretKey      string `mapstructure:"S3_SECRET_KEY"`
	S3ForcePathStyle bool   `mapstructure:"S3_FORCE_PATH_STYLE"`

	// ── JWT Auth ─────────────────────────────────────────────────
	JWTSecret            string        `mapstructure:"JWT_SECRET"`
	AccessTokenDuration  time.Duration `mapstructure:"JWT_ACCESS_TOKEN_TTL"`
	RefreshTokenDuration time.Duration `mapstructure:"JWT_REFRESH_TOKEN_TTL"`
	JWTIssuer            string        `mapstructure:"JWT_ISSUER"`

	// ── Encryption ───────────────────────────────────────────────
	EncryptionKey string `mapstructure:"ENCRYPTION_KEY"`

	// ── CORS ─────────────────────────────────────────────────────
	CORSAllowedOrigins   string   `mapstructure:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool     `mapstructure:"CORS_ALLOW_CREDENTIALS"`
	AllowedOrigins       []string // parsed from CORSAllowedOrigins

	// ── Email (SMTP — MailHog in dev, smtp.resend.com in prod) ────
	EmailFrom    string `mapstructure:"EMAIL_FROM"`
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUsername string `mapstructure:"SMTP_USERNAME"` // "resend" when using Resend relay
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"` // Resend API key

	// ── Web Push (VAPID) ─────────────────────────────────────────
	VAPIDPublicKey  string `mapstructure:"VAPID_PUBLIC_KEY"`
	VAPIDPrivateKey string `mapstructure:"VAPID_PRIVATE_KEY"`
	VAPIDEmail      string `mapstructure:"VAPID_EMAIL"` // e.g. mailto:admin@cashpilotx.app

	// ── Digest scheduler ─────────────────────────────────────────
	DigestHourUTC int    `mapstructure:"DIGEST_HOUR_UTC"` // 0-23, default 8
	AppURL        string `mapstructure:"APP_URL"`         // e.g. https://cashpilotx.app
}

// LoadConfig loads the configuration from the given path.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	mustBindEnv(
		"APP_ENV", "DATABASE_URL", "REDIS_URL",
		"S3_BUCKET", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_REGION",
		"JWT_SECRET", "JWT_ACCESS_TOKEN_TTL", "JWT_REFRESH_TOKEN_TTL",
		"CORS_ALLOWED_ORIGINS",
	)

	viper.SetDefault("APP_ENV", EnvironmentDevelopment)
	viper.SetDefault("SERVER_HOST", "0.0.0.0")
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("SERVER_READ_TIMEOUT", "15s")
	viper.SetDefault("SERVER_WRITE_TIMEOUT", "15s")
	viper.SetDefault("SERVER_IDLE_TIMEOUT", "60s")
	viper.SetDefault("JWT_ACCESS_TOKEN_TTL", "15m")
	viper.SetDefault("JWT_REFRESH_TOKEN_TTL", "168h")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("REDIS_PORT", 6379)
	viper.SetDefault("JWT_SECRET", utils.RandomString(32))
	viper.SetDefault("DATABASE_URL", "")
	viper.SetDefault("REDIS_URL", "")
	viper.SetDefault("S3_BUCKET", "")
	viper.SetDefault("S3_ENDPOINT", "")
	viper.SetDefault("S3_ACCESS_KEY", "")
	viper.SetDefault("S3_SECRET_KEY", "")
	viper.SetDefault("S3_REGION", "")
	viper.SetDefault("EMAIL_FROM", "noreply@cashpilotx.local")
	viper.SetDefault("SMTP_HOST", "localhost")
	viper.SetDefault("SMTP_PORT", 1025)
	viper.SetDefault("SMTP_USERNAME", "")
	viper.SetDefault("SMTP_PASSWORD", "")
	viper.SetDefault("VAPID_PUBLIC_KEY", "")
	viper.SetDefault("VAPID_PRIVATE_KEY", "")
	viper.SetDefault("VAPID_EMAIL", "mailto:admin@cashpilotx.app")
	viper.SetDefault("DIGEST_HOUR_UTC", 8)
	viper.SetDefault("APP_URL", "https://cashpilotx.app")

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
