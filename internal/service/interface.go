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
