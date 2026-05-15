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
	Auth     AuthServicer
	Customer CustomerServicer
	Invoice  InvoiceServicer
	Payment  PaymentServicer
	Message  MessageServicer
	Reminder ReminderServicer
	Audit    AuditServicer
	Sync     SyncServicer
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

type InvoiceService struct {
	store db.Store
}

type PaymentService struct {
	store db.Store
}

type MessageService struct {
	store db.Store
}

type ReminderService struct {
	store db.Store
}

type AuditService struct {
	store db.Store
}

type CustomerService struct {
	store  db.Store
	encKey []byte // 32-byte AES-256-GCM key for channel config encryption
}

type CustomerConfig struct {
	// ChannelEncryptionKey must be exactly 32 bytes for AES-256-GCM.
	ChannelEncryptionKey []byte
}

type Config struct {
	Auth     AuthConfig
	Customer CustomerConfig
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
		Auth:     *DefaultAuthConfig(),
		Customer: CustomerConfig{},
	}
}

func NewInvoiceService(store db.Store) *InvoiceService {
	return &InvoiceService{store: store}
}

func NewPaymentService(store db.Store) *PaymentService {
	return &PaymentService{store: store}
}

func NewMessageService(store db.Store) *MessageService {
	return &MessageService{store: store}
}

func NewReminderService(store db.Store) *ReminderService {
	return &ReminderService{store: store}
}

func NewAuditService(store db.Store) *AuditService {
	return &AuditService{store: store}
}

func NewCustomerService(store db.Store, cfg *CustomerConfig) *CustomerService {
	var key []byte
	if cfg != nil {
		key = cfg.ChannelEncryptionKey
	}
	return &CustomerService{store: store, encKey: key}
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
	customerSvc := NewCustomerService(store, &cfg.Customer)
	invoiceSvc := NewInvoiceService(store)
	paymentSvc := NewPaymentService(store)
	return &Services{
		Auth:     authSvc,
		Customer: customerSvc,
		Invoice:  invoiceSvc,
		Payment:  paymentSvc,
		Message:  NewMessageService(store),
		Reminder: NewReminderService(store),
		Audit:    NewAuditService(store),
		Sync:     NewSyncService(store),
	}
}
