package service

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/token"
	"context"

	"github.com/google/uuid"
)

type AuthServicer interface {
	Register(ctx context.Context, req *dto.RegisterRequest, ipAddress, userAgent string) (*dto.RegisterResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest, ipAddress, userAgent string) (*dto.LoginResponse, error)
	Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest, ipAddress, userAgent string) (*dto.RefreshTokenResponse, error)
	ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error)
	GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error)
	RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error
	ForgotPIN(ctx context.Context, req *dto.ForgotPINRequest) error
	ResetPIN(ctx context.Context, req *dto.ResetPINRequest) error
}

type CustomerServicer interface {
	// Customer CRUD
	CreateCustomer(ctx context.Context, userID uuid.UUID, req *dto.CreateCustomerRequest) (*dto.CustomerResponse, error)
	GetCustomer(ctx context.Context, userID, customerID uuid.UUID) (*dto.CustomerResponse, error)
	ListCustomers(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.CustomerResponse, error)
	SearchCustomers(ctx context.Context, userID uuid.UUID, query string) ([]*dto.CustomerResponse, error)
	UpdateCustomer(ctx context.Context, userID, customerID uuid.UUID, req *dto.UpdateCustomerRequest) (*dto.CustomerResponse, error)
	ArchiveCustomer(ctx context.Context, userID, customerID uuid.UUID) error

	// Channel configs (per-user, not per-customer)
	UpsertChannelConfig(ctx context.Context, userID uuid.UUID, req *dto.UpsertChannelConfigRequest) (*dto.ChannelConfigResponse, error)
	ListChannelConfigs(ctx context.Context, userID uuid.UUID) ([]*dto.ChannelConfigResponse, error)
	GetChannelConfig(ctx context.Context, userID uuid.UUID, channel string) (*dto.ChannelConfigResponse, error)
	DeleteChannelConfig(ctx context.Context, userID uuid.UUID, channel string) error
	SetChannelActive(ctx context.Context, userID uuid.UUID, channel string, active bool) error
}
