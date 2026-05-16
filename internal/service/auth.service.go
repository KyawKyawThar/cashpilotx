// Package service implements the business logic for the CashPilotX application.
package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/token"
	"cashpilotx/utils"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// generateRefreshToken creates a cryptographically random 64-character hex string.
// Opaque tokens avoid the chicken-and-egg problem of needing a user ID before
// RegisterUserTx has committed the new user row.
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *AuthService) RequestEmailVerification(ctx context.Context, email string) error {
	otp, err := generateOTP()
	if err != nil {
		return api.ErrInternal(fmt.Errorf("generate OTP: %w", err))
	}
	if err := s.cache.StoreRegOTP(ctx, email, otp); err != nil {
		return api.ErrInternal(fmt.Errorf("store reg OTP: %w", err))
	}
	if err := s.emailSender.SendRegistrationOTP(ctx, email, otp); err != nil {
		return api.ErrInternal(fmt.Errorf("send verification email: %w", err))
	}
	return nil
}

func (s *AuthService) Register(
	ctx context.Context,
	req *dto.RegisterRequest,
	ipAddress, userAgent string,
) (*dto.RegisterResponse, error) {
	if req.PIN != req.ConfirmPIN {
		return nil, api.ErrValidation("PIN and confirm PIN do not match")
	}

	storedOTP, err := s.cache.GetRegOTP(ctx, req.Email)
	if err != nil {
		if errors.Is(err, cache.ErrTokenNotFound) {
			return nil, api.ErrInvalidOTP()
		}
		return nil, api.ErrInternal(fmt.Errorf("get reg OTP: %w", err))
	}
	if storedOTP != req.EmailOTP {
		return nil, api.ErrInvalidOTP()
	}
	s.cache.DeleteRegOTP(ctx, req.Email) //nolint:errcheck // best-effort cleanup; OTP already validated and consumed above

	pinHash, err := utils.HashPassword(req.PIN)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("hash PIN: %w", err))
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("generate refresh token: %w", err))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	email := req.Email
	result, err := s.store.RegisterUserTx(dbCtx, db.RegisterUserTxParams{
		User: db.CreateUserParams{
			Phone:        req.Phone,
			PinHash:      pinHash,
			Name:         req.Name,
			BusinessName: req.BusinessName,
			Plan:         "free",
			HomeCurrency: req.HomeCurrency,
			Email:        &email,
		},
		Session: db.CreateSessionParams{
			RefreshToken: refreshToken,
			UserAgent:    &userAgent,
			IpAddress:    &ipAddress,
			ExpiresAt:    time.Now().Add(s.config.RefreshTokenDuration),
		},
	})
	if err != nil {
		if api.IsUniqueViolation(err) {
			msg := err.Error()
			if strings.Contains(msg, "email") {
				return nil, api.ErrEmailAlreadyExists()
			}
			return nil, api.ErrPhoneAlreadyExists()
		}
		return nil, api.FromPgError(err)
	}

	accessToken, _, err := s.tokenMaker.CreateToken(result.User.ID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	s.cacheSession(ctx, result.Session)

	return &dto.RegisterResponse{
		User:         dto.ToUserResponse(result.User),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req *dto.LoginRequest,
	ipAddress, userAgent string,
) (*dto.LoginResponse, error) {
	// Cache check — no DB budget consumed yet.
	locked, lockedUntil, err := s.cache.IsLoginLocked(ctx, req.Phone)
	if err == nil && locked {
		if lockedUntil != nil {
			return nil, api.ErrLoginLocked(*lockedUntil)
		}
		return nil, api.ErrLoginLocked(time.Now().Add(5 * time.Minute))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	user, err := s.store.GetUserByPhone(dbCtx, req.Phone)
	if err != nil {
		if api.IsRecordNotFound(err) {
			// Unknown phone — record the failed attempt so brute-force lockout applies.
			s.cache.RecordLoginAttempt(ctx, req.Phone, false) //nolint:errcheck // best-effort rate-limit tracking; a cache failure does not affect login correctness
			return nil, api.ErrInvalidCredentials()
		}
		// DB timeout / connection failure — don't penalize the user's lockout counter.
		return nil, api.FromPgError(err)
	}
	if !user.IsActive {
		return nil, api.ErrForbidden("Account has been deactivated")
	}

	if err = utils.CheckPassword(req.PIN, user.PinHash); err != nil {
		result, _ := s.cache.RecordLoginAttempt(ctx, req.Phone, false) //nolint:errcheck // on cache error result is nil; fallback is to allow the attempt
		if result != nil && !result.Allowed && result.LockoutDuration > 0 {
			return nil, api.ErrLoginLocked(time.Now().Add(result.LockoutDuration))
		}
		return nil, api.ErrInvalidCredentials()
	}

	s.cache.RecordLoginAttempt(ctx, req.Phone, true) //nolint:errcheck // best-effort rate-limit reset; a cache failure does not affect a successful login

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("generate refresh token: %w", err))
	}

	rotateResult, err := s.store.RotateSessionTx(dbCtx, db.RotateSessionTxParams{
		Session: db.CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: refreshToken,
			UserAgent:    &userAgent,
			IpAddress:    &ipAddress,
			ExpiresAt:    time.Now().Add(s.config.RefreshTokenDuration),
		},
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	accessToken, _, err := s.tokenMaker.CreateToken(user.ID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	s.cacheSession(ctx, rotateResult.Session)

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
		User:         dto.ToUserResponse(user),
	}, nil
}

func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *dto.RefreshTokenRequest,
	ipAddress, userAgent string,
) (*dto.RefreshTokenResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	session, err := s.store.GetSessionByRefreshToken(dbCtx, req.RefreshToken)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrSessionExpired()
		}
		return nil, api.FromPgError(err)
	}

	user, err := s.store.GetUserByID(dbCtx, session.UserID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvalidCredentials()
		}
		return nil, api.FromPgError(err)
	}
	if !user.IsActive {
		return nil, api.ErrForbidden("Account has been deactivated")
	}

	// Consume the old session before issuing a new one (token rotation).
	if _, deleteErr := s.store.DeleteSessionByRefreshToken(dbCtx, req.RefreshToken); deleteErr != nil {
		return nil, api.FromPgError(deleteErr)
	}
	s.cache.DeleteSession(ctx, session.ID.String(), session.UserID.String()) //nolint:errcheck // best-effort cache eviction; DB deletion already succeeded

	newRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("generate refresh token: %w", err))
	}

	rotateResult, err := s.store.RotateSessionTx(dbCtx, db.RotateSessionTxParams{
		Session: db.CreateSessionParams{
			UserID:       session.UserID,
			RefreshToken: newRefreshToken,
			UserAgent:    &userAgent,
			IpAddress:    &ipAddress,
			ExpiresAt:    time.Now().Add(s.config.RefreshTokenDuration),
		},
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	accessToken, _, err := s.tokenMaker.CreateToken(session.UserID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	s.cacheSession(ctx, rotateResult.Session)

	return &dto.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error {
	payload, err := s.tokenMaker.VerifyToken(accessToken)
	if err != nil {
		if errors.Is(err, token.ErrInvalidToken) || errors.Is(err, token.ErrTokenExpired) {
			return nil // Already invalid — treat as success
		}
		return api.ErrInternal(fmt.Errorf("verify token: %w", err))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if req.LogoutAll {
		if err := s.store.DeleteAllUserSessions(dbCtx, payload.UserID); err != nil {
			return api.FromPgError(err)
		}
		// Cache eviction is best-effort: sessions are already gone from DB.
		// A Redis failure here must not make the client think logout failed.
		_ = s.cache.DeleteAllUserSessions(ctx, payload.UserID.String()) //nolint:errcheck // best-effort cache eviction; DB sessions are already deleted
		return nil
	}

	if req.SessionID != "" {
		sessUUID, err := uuid.Parse(req.SessionID)
		if err != nil {
			return api.ErrBadRequest("session_id must be a valid UUID")
		}
		if _, err := s.store.DeleteSessionByID(dbCtx, sessUUID); err != nil {
			if api.IsRecordNotFound(err) {
				return api.ErrNotFound("Session")
			}
			return api.FromPgError(err)
		}
		s.cache.DeleteSession(ctx, req.SessionID, payload.UserID.String()) //nolint:errcheck // best-effort cache eviction; DB deletion already succeeded
		return nil
	}

	// Default: blacklist the current access token JTI (cache-only, no DB call).
	s.cache.BlacklistToken(ctx, payload.ID.String(), payload.ExpiredAt) //nolint:errcheck // best-effort blacklist; token expires naturally on its TTL
	return nil
}

func (s *AuthService) ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error) {
	payload, err := s.tokenMaker.VerifyToken(tokenStr)
	if err != nil {
		return nil, api.ErrInvalidToken("")
	}

	blacklisted, err := s.cache.IsTokenBlacklisted(ctx, payload.ID.String())
	if err == nil && blacklisted {
		return nil, api.ErrInvalidToken("Token has been revoked")
	}

	return payload, nil
}

func (s *AuthService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error) {
	// Fast path: Redis (no DB budget needed).
	cacheSessions, err := s.cache.GetUserSessions(ctx, userID.String())
	if err == nil && len(cacheSessions) > 0 {
		out := make([]*dto.SessionResponse, 0, len(cacheSessions))
		for _, cs := range cacheSessions {
			out = append(out, dto.CacheSessionToResponse(cs))
		}
		return out, nil
	}

	// Fallback: DB (e.g. after a Redis flush).
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	dbSessions, err := s.store.ListUserSessions(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.SessionResponse, 0, len(dbSessions))
	for _, ds := range dbSessions {
		out = append(out, dto.DbSessionToResponse(ds))
	}
	return out, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return api.ErrBadRequest("session_id must be a valid UUID")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.store.DeleteSessionByID(dbCtx, sessUUID); err != nil {
		if api.IsRecordNotFound(err) {
			// Session may exist only in cache (e.g. after a DB reset or cache-ahead write).
			// Evict it so it cannot be listed again; if it's not in cache either, 404.
			if _, cerr := s.cache.GetSession(ctx, sessionID); cerr != nil {
				return api.ErrNotFound("Session")
			}
			s.cache.DeleteSession(ctx, sessionID, userID.String()) //nolint:errcheck // best-effort cache eviction; session treated as not-found
			return nil
		}
		return api.FromPgError(err)
	}
	s.cache.DeleteSession(ctx, sessionID, userID.String()) //nolint:errcheck // best-effort cache eviction; DB deletion already succeeded
	return nil
}

// generateOTP creates a cryptographically random 6-digit numeric code.
func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// lookupUserByIdentifier resolves a phone-or-email string to a DB user.
func (s *AuthService) lookupUserByIdentifier(ctx context.Context, identifier string) (db.User, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if strings.Contains(identifier, "@") {
		user, err := s.store.GetUserByEmail(dbCtx, &identifier)
		if err != nil {
			if api.IsRecordNotFound(err) {
				return db.User{}, api.ErrInvalidCredentials()
			}
			return db.User{}, api.FromPgError(err)
		}
		return user, nil
	}

	user, err := s.store.GetUserByPhone(dbCtx, identifier)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return db.User{}, api.ErrInvalidCredentials()
		}
		return db.User{}, api.FromPgError(err)
	}
	return user, nil
}

func (s *AuthService) ForgotPIN(ctx context.Context, req *dto.ForgotPINRequest) error {
	user, err := s.lookupUserByIdentifier(ctx, req.Identifier)
	if err != nil {
		// Return success even on not-found to prevent user enumeration.
		return nil //nolint:nilerr // Return success even on not-found to prevent user enumeration.
	}

	if user.Email == nil || *user.Email == "" {
		return nil // no email registered — silently succeed to prevent enumeration
	}

	return s.sendPINResetEmail(ctx, user.ID.String(), *user.Email, user.Name)
}

func (s *AuthService) ResetPIN(ctx context.Context, req *dto.ResetPINRequest) error {
	if req.NewPIN != req.ConfirmPIN {
		return api.ErrValidation("new PIN and confirm PIN do not match")
	}

	user, err := s.lookupUserByIdentifier(ctx, req.Identifier)
	if err != nil {
		return err
	}

	storedOTP, err := s.cache.GetOTP(ctx, user.ID.String())
	if err != nil {
		if errors.Is(err, cache.ErrTokenNotFound) {
			return api.ErrInvalidOTP()
		}
		return api.ErrInternal(fmt.Errorf("get OTP: %w", err))
	}

	if storedOTP != req.OTP {
		return api.ErrInvalidOTP()
	}

	pinHash, err := utils.HashPassword(req.NewPIN)
	if err != nil {
		return api.ErrInternal(fmt.Errorf("hash PIN: %w", err))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.store.UpdateUserPIN(dbCtx, db.UpdateUserPINParams{
		ID:      user.ID,
		PinHash: pinHash,
	}); err != nil {
		return api.FromPgError(err)
	}

	// Consume the OTP and evict all sessions so the user must log in fresh.
	s.cache.DeleteOTP(ctx, user.ID.String())             //nolint:errcheck // Best-effort cleanup; OTP is single-use and now invalid.
	s.cache.DeleteAllUserSessions(ctx, user.ID.String()) //nolint:errcheck // Best-effort cleanup; user will log in fresh.
	s.store.DeleteAllUserSessions(dbCtx, user.ID)        //nolint:errcheck // Best-effort cleanup; user will log in fresh.

	return nil
}

// DeleteUser cleans up all Redis keys for the user, then hard-deletes the DB row.
// All child rows (sessions, customers, invoices, etc.) cascade automatically.
func (s *AuthService) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	user, err := s.store.GetUserByID(dbCtx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrUserNotFound()
		}
		return api.FromPgError(err)
	}

	// Clear rate-limit keys keyed on the phone number before the row disappears.
	s.cache.ClearLoginAttempts(ctx, user.Phone) //nolint:errcheck // best-effort; Redis cleanup does not affect correctness of the DB delete

	// Clear all session keys from Redis; DB sessions cascade from the DELETE below.
	s.cache.DeleteAllUserSessions(ctx, userID.String()) //nolint:errcheck // best-effort; DB delete is the authoritative operation

	return api.FromPgError(s.store.DeleteUser(dbCtx, userID))
}

// GetMe returns the authenticated user's profile.
func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	user, err := s.store.GetUserByID(dbCtx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}
	return dto.ToUserResponse(user), nil
}

// UpdateHomeCurrency persists the user's preferred home currency.
func (s *AuthService) UpdateHomeCurrency(ctx context.Context, userID uuid.UUID, currency string) (*dto.UserResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	user, err := s.store.UpdateHomeCurrency(dbCtx, db.UpdateHomeCurrencyParams{
		ID:           userID,
		HomeCurrency: currency,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}
	return dto.ToUserResponse(user), nil
}
