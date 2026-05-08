// Package service implements the business logic for the CashPilotX application.
package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/token"
	"cashpilotx/utils"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

func (s *AuthService) Register(
	ctx context.Context,
	req *dto.RegisterRequest,
	ipAddress, userAgent string,
) (*dto.RegisterResponse, error) {
	if req.PIN != req.ConfirmPIN {
		return nil, api.ErrValidation("PIN and confirm PIN do not match")
	}

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

	result, err := s.store.RegisterUserTx(dbCtx, db.RegisterUserTxParams{
		User: db.CreateUserParams{
			Phone:        req.Phone,
			PinHash:      pinHash,
			Name:         req.Name,
			BusinessName: req.BusinessName,
			Plan:         "free",
			HomeCurrency: req.HomeCurrency,
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
