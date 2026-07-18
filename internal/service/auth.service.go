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
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	loginTypePhone    = "phone"
	loginTypeUsername = "username"
)

var phonePattern = regexp.MustCompile(`^\+?\d{8,15}$`)

// isPhoneIdentifier returns true if the string looks like a phone number.
func isPhoneIdentifier(s string) bool {
	return phonePattern.MatchString(s)
}

// localeFromCurrency maps a home_currency code to its language tag.
// MMK → my (Burmese), CNY → zh (Chinese), THB and anything else → th (Thai).
func localeFromCurrency(currency string) string {
	switch currency {
	case "MMK":
		return "my"
	case "CNY":
		return "zh"
	default:
		return "th"
	}
}

// generateRefreshToken creates a cryptographically random 64-character hex string.
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
	s.cache.DeleteRegOTP(ctx, req.Email) //nolint:errcheck // best-effort cleanup

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
	phone := req.Phone
	locale := localeFromCurrency(req.HomeCurrency)
	result, err := s.store.RegisterUserTx(dbCtx, db.RegisterUserTxParams{
		User: db.CreateUserParams{
			Phone:        &phone,
			PinHash:      pinHash,
			Name:         req.Name,
			BusinessName: &req.BusinessName,
			Locale:       &locale,
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

	accessToken, _, err := s.tokenMaker.CreateToken(tokenParamsFromUser(result.User, result.User.Plan, s.config.AccessTokenDuration, loginTypePhone))
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
	// Rate-limit check keyed on the identifier (phone or username).
	locked, lockedUntil, err := s.cache.IsLoginLocked(ctx, req.Identifier)
	if err == nil && locked {
		if lockedUntil != nil {
			return nil, api.ErrLoginLocked(*lockedUntil)
		}
		return nil, api.ErrLoginLocked(time.Now().Add(5 * time.Minute))
	}

	user, loginType, err := s.lookupUserForLogin(ctx, req.Identifier)
	if err != nil {
		s.cache.RecordLoginAttempt(ctx, req.Identifier, false) //nolint:errcheck // best-effort
		return nil, err
	}
	if !user.IsActive {
		return nil, api.ErrForbidden("Account has been deactivated")
	}

	loginDBCtx, loginCancel := withDBTimeout(ctx)
	effectivePlan, err := s.resolveEffectivePlan(loginDBCtx, user)
	loginCancel()
	if err != nil {
		return nil, err
	}

	if err = utils.CheckPassword(req.PIN, user.PinHash); err != nil {
		result, _ := s.cache.RecordLoginAttempt(ctx, req.Identifier, false) //nolint:errcheck // best-effort rate-limit tracking
		if result != nil && !result.Allowed && result.LockoutDuration > 0 {
			return nil, api.ErrLoginLocked(time.Now().Add(result.LockoutDuration))
		}
		return nil, api.ErrInvalidCredentials()
	}

	s.cache.RecordLoginAttempt(ctx, req.Identifier, true) //nolint:errcheck // best-effort reset

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("generate refresh token: %w", err))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

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

	accessToken, _, err := s.tokenMaker.CreateToken(tokenParamsFromUser(user, effectivePlan, s.config.AccessTokenDuration, loginType))
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

	// AnyStatus so a deactivated account hits the explicit is_active check
	// below with its specific error, rather than looking indistinguishable
	// from a stale/invalid session — see lookupUserForLogin.
	user, err := s.store.GetUserByIDAnyStatus(dbCtx, session.UserID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvalidCredentials()
		}
		return nil, api.FromPgError(err)
	}
	if !user.IsActive {
		return nil, api.ErrForbidden("Account has been deactivated")
	}

	effectivePlan, err := s.resolveEffectivePlan(dbCtx, user)
	if err != nil {
		return nil, err
	}

	if _, deleteErr := s.store.DeleteSessionByRefreshToken(dbCtx, req.RefreshToken); deleteErr != nil {
		return nil, api.FromPgError(deleteErr)
	}
	s.cache.DeleteSession(ctx, session.ID.String(), session.UserID.String()) //nolint:errcheck // best-effort cache cleanup

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

	// Derive login type from user data so the refreshed token stays consistent.
	loginType := loginTypePhone
	if user.Username != nil {
		loginType = loginTypeUsername
	}

	accessToken, _, err := s.tokenMaker.CreateToken(tokenParamsFromUser(user, effectivePlan, s.config.AccessTokenDuration, loginType))
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
			return nil
		}
		return api.ErrInternal(fmt.Errorf("verify token: %w", err))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if req.LogoutAll {
		if err := s.store.DeleteAllUserSessions(dbCtx, payload.UserID); err != nil {
			return api.FromPgError(err)
		}
		_ = s.cache.DeleteAllUserSessions(ctx, payload.UserID.String()) //nolint:errcheck // best-effort cache cleanup
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
		s.cache.DeleteSession(ctx, req.SessionID, payload.UserID.String()) //nolint:errcheck // best-effort cache cleanup
		return nil
	}

	s.cache.BlacklistToken(ctx, payload.ID.String(), payload.ExpiredAt) //nolint:errcheck // best-effort token blacklist
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
	cacheSessions, err := s.cache.GetUserSessions(ctx, userID.String())
	if err == nil && len(cacheSessions) > 0 {
		out := make([]*dto.SessionResponse, 0, len(cacheSessions))
		for _, cs := range cacheSessions {
			out = append(out, dto.CacheSessionToResponse(cs))
		}
		return out, nil
	}

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
			if _, cerr := s.cache.GetSession(ctx, sessionID); cerr != nil {
				return api.ErrNotFound("Session")
			}
			s.cache.DeleteSession(ctx, sessionID, userID.String()) //nolint:errcheck // best-effort cache cleanup
			return nil
		}
		return api.FromPgError(err)
	}
	s.cache.DeleteSession(ctx, sessionID, userID.String()) //nolint:errcheck // best-effort cache cleanup
	return nil
}

// tokenParamsFromUser builds CreateTokenParams from a db.User row. plan is
// passed explicitly rather than read from user.Plan because team members
// carry a stale plan snapshot from account creation — callers must resolve
// the org owner's current plan via resolveEffectivePlan first.
func tokenParamsFromUser(user db.User, plan string, duration time.Duration, loginType string) token.CreateTokenParams {
	p := token.CreateTokenParams{
		UserID:         user.ID,
		Duration:       duration,
		ForcePinChange: user.ForcePinChange,
		LoginType:      loginType,
		Plan:           plan,
		Role:           string(user.Role),
		TeamID:         user.TeamID,
		OrgOwnerID:     user.OrgOwnerID,
	}
	return p
}

// resolveEffectivePlan returns the plan that should gate this user's access.
// Team members carry their own plan/is_active columns as a snapshot from
// account creation, but billing lives on the org owner — so a plan
// downgrade or deactivation on the owner must lock out the whole team
// immediately, without a cascading write to every member row. For the
// owner (OrgOwnerID == own ID, or unset on legacy rows) this is a no-op.
func (s *AuthService) resolveEffectivePlan(ctx context.Context, user db.User) (string, error) {
	if user.OrgOwnerID == nil || *user.OrgOwnerID == user.ID {
		return user.Plan, nil
	}
	owner, err := s.store.GetUserByID(ctx, *user.OrgOwnerID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return "", api.ErrForbidden("Your organization's account has been deactivated")
		}
		return "", api.FromPgError(err)
	}
	return owner.Plan, nil
}

// generateOTP creates a cryptographically random 6-digit numeric code.
func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// lookupUserForLogin resolves an identifier for the login flow only.
// Phone pattern → super_admin via GetUserByPhone.
// Anything else → team account via GetUserByUsername.
// Returns the user and the login type ("phone" or "username").
func (s *AuthService) lookupUserForLogin(ctx context.Context, identifier string) (db.User, string, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// AnyStatus variants: a deactivated account must still be found here so
	// the caller's explicit is_active check can return the specific
	// "deactivated" error, instead of falling through to the generic
	// "invalid credentials" message a true not-found produces below.
	if isPhoneIdentifier(identifier) {
		user, err := s.store.GetUserByPhoneAnyStatus(dbCtx, &identifier)
		if err != nil {
			if api.IsRecordNotFound(err) {
				return db.User{}, "", api.ErrInvalidCredentials()
			}
			return db.User{}, "", api.FromPgError(err)
		}
		return user, loginTypePhone, nil
	}

	user, err := s.store.GetUserByUsernameAnyStatus(dbCtx, &identifier)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return db.User{}, "", api.ErrInvalidCredentials()
		}
		return db.User{}, "", api.FromPgError(err)
	}
	return user, loginTypeUsername, nil
}

// lookupUserByIdentifier resolves phone or email — used by ForgotPIN / ResetPIN flows.
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

	user, err := s.store.GetUserByPhone(dbCtx, &identifier)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return db.User{}, api.ErrInvalidCredentials()
		}
		return db.User{}, api.FromPgError(err)
	}
	return user, nil
}

func (s *AuthService) ForgotPIN(ctx context.Context, req *dto.ForgotPINRequest) error {
	// Username identifiers belong to team accounts — they cannot self-reset via OTP.
	if !isPhoneIdentifier(req.Identifier) && !strings.Contains(req.Identifier, "@") {
		return api.ErrForbidden("Contact your administrator to reset your PIN")
	}

	user, err := s.lookupUserByIdentifier(ctx, req.Identifier)
	if err != nil {
		// Silently succeed on not-found to prevent user enumeration.
		return nil //nolint:nilerr // intentional: silently succeed to prevent user enumeration
	}

	if user.Email == nil || *user.Email == "" {
		return nil // no email registered — silently succeed
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

	s.cache.DeleteOTP(ctx, user.ID.String())             //nolint:errcheck // best-effort cache cleanup
	s.cache.DeleteAllUserSessions(ctx, user.ID.String()) //nolint:errcheck // best-effort cache cleanup
	s.store.DeleteAllUserSessions(dbCtx, user.ID)        //nolint:errcheck // best-effort DB cleanup after PIN reset

	return nil
}

// ChangePIN verifies the current PIN, updates it, clears force_pin_change, and
// returns a fresh access token so the client session is immediately up to date.
// Only super_admin (phone-login) accounts may change their own PIN; team accounts
// must have their PIN reset by a super_admin via the admin panel.
func (s *AuthService) ChangePIN(ctx context.Context, userID uuid.UUID, req *dto.ChangePINRequest) (*dto.ChangePINResponse, error) {
	if req.NewPIN != req.ConfirmPIN {
		return nil, api.ErrValidation("new PIN and confirm PIN do not match")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	user, err := s.store.GetUserByID(dbCtx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}

	if user.Role != db.UserRoleSuperAdmin {
		return nil, api.ErrForbidden("team accounts cannot change their own PIN — ask your super admin to reset it")
	}

	if checkErr := utils.CheckPassword(req.CurrentPIN, user.PinHash); checkErr != nil {
		return nil, api.ErrInvalidCredentials()
	}

	pinHash, err := utils.HashPassword(req.NewPIN)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("hash PIN: %w", err))
	}

	if updateErr := s.store.UpdateUserPINAndClearForce(dbCtx, db.UpdateUserPINAndClearForceParams{
		ID:      userID,
		PinHash: pinHash,
	}); updateErr != nil {
		return nil, api.FromPgError(updateErr)
	}

	loginType := loginTypePhone
	if user.Username != nil {
		loginType = loginTypeUsername
	}

	// force_pin_change is already cleared by UpdateUserPINAndClearForce above.
	user.ForcePinChange = false
	accessToken, _, err := s.tokenMaker.CreateToken(tokenParamsFromUser(user, user.Plan, s.config.AccessTokenDuration, loginType))
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	return &dto.ChangePINResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:   "Bearer",
	}, nil
}

// DeleteUser cleans up all Redis keys for the user, then hard-deletes the DB row.
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

	// Clear rate-limit keys keyed on the identifier (phone for super_admin, username for team).
	if user.Phone != nil {
		s.cache.ClearLoginAttempts(ctx, *user.Phone) //nolint:errcheck // best-effort cache cleanup
	}
	if user.Username != nil {
		s.cache.ClearLoginAttempts(ctx, *user.Username) //nolint:errcheck // best-effort cache cleanup
	}

	s.cache.DeleteAllUserSessions(ctx, userID.String()) //nolint:errcheck // best-effort cache cleanup

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
