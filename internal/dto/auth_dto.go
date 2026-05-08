// Package dto defines request/response data transfer objects for the CashPilotX API.
package dto

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"time"

	"github.com/google/uuid"
)

type MessageResponse struct {
	Message string `json:"message"`
}

func NewMessageResponse(msg string) *MessageResponse {
	return &MessageResponse{Message: msg}
}

// RegisterRequest carries fields for new user registration (phone + email + PIN).
type RegisterRequest struct {
	Phone        string  `json:"phone"         validate:"required,phone"`
	Email        string  `json:"email"         validate:"required,email"`
	PIN          string  `json:"pin"           validate:"required,pin"`
	ConfirmPIN   string  `json:"confirm_pin"   validate:"required,pin"`
	Name         string  `json:"name"          validate:"required,min=2,max=100"`
	BusinessName *string `json:"business_name" validate:"omitempty,min=2,max=100"`
	HomeCurrency string  `json:"home_currency" validate:"required,currency"`
}

// RegisterResponse is returned after successful registration.
type RegisterResponse struct {
	User         *UserResponse `json:"user"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    int64         `json:"expires_in"`
	TokenType    string        `json:"token_type"`
}

// LoginRequest carries phone and PIN for authentication.
type LoginRequest struct {
	Phone string `json:"phone" validate:"required,phone"`
	PIN   string `json:"pin"   validate:"required,pin"`
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    int64         `json:"expires_in"`
	TokenType    string        `json:"token_type"`
	User         *UserResponse `json:"user"`
}

// RefreshTokenRequest carries the opaque refresh token.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshTokenResponse is returned after a successful token refresh.
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// ForgotPINRequest accepts either a phone number or email address to identify the user.
type ForgotPINRequest struct {
	Identifier string `json:"identifier" validate:"required,min=3,max=254"`
}

// ResetPINRequest carries the OTP and the new PIN chosen by the user.
type ResetPINRequest struct {
	Identifier string `json:"identifier"  validate:"required,min=3,max=254"`
	OTP        string `json:"otp"         validate:"required,len=6"`
	NewPIN     string `json:"new_pin"     validate:"required,pin"`
	ConfirmPIN string `json:"confirm_pin" validate:"required,pin"`
}

// LogoutRequest optionally targets a specific session or all sessions.
type LogoutRequest struct {
	SessionID string `json:"session_id,omitempty"` // specific session; empty = current token only
	LogoutAll bool   `json:"logout_all"`           // true = revoke all sessions
}

// SessionResponse represents a single active session.
type SessionResponse struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// UserResponse is the public representation of a CashPilotX user.
type UserResponse struct {
	ID           uuid.UUID `json:"id"`
	Phone        string    `json:"phone"`
	Name         string    `json:"name"`
	BusinessName *string   `json:"business_name,omitempty"`
	HomeCurrency string    `json:"home_currency"`
	Plan         string    `json:"plan"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// ToUserResponse maps db.User to a client-facing UserResponse.
func ToUserResponse(u db.User) *UserResponse {
	return &UserResponse{
		ID:           u.ID,
		Phone:        u.Phone,
		Name:         u.Name,
		BusinessName: u.BusinessName,
		HomeCurrency: u.HomeCurrency,
		Plan:         u.Plan,
		IsActive:     u.IsActive,
		CreatedAt:    u.CreatedAt,
	}
}

// CacheSessionToResponse converts a cached session into a SessionResponse.
func CacheSessionToResponse(s *cache.Session) *SessionResponse {
	return &SessionResponse{
		ID:           s.ID,
		UserAgent:    s.UserAgent,
		IPAddress:    s.IPAddress,
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		LastActiveAt: s.LastActiveAt,
	}
}

// DbSessionToResponse converts a database session row into a SessionResponse.
func DbSessionToResponse(s db.Session) *SessionResponse {
	r := &SessionResponse{
		ID:           s.ID.String(),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		LastActiveAt: s.CreatedAt, // db.Session has no LastUsedAt; seed from CreatedAt
	}
	if s.UserAgent != nil {
		r.UserAgent = *s.UserAgent
	}
	if s.IpAddress != nil {
		r.IPAddress = *s.IpAddress
	}
	return r
}
