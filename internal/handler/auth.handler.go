// Package handler implements HTTP request handlers for the CashPilotX API.
package handler

import (
	"cashpilotx/internal/dto"
	"net/http"
)

// RequestEmailVerification godoc
//
//	@Summary      Send registration email OTP
//	@Description  Send a 6-digit OTP to the provided email to verify ownership before account creation.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.RequestEmailVerificationRequest  true  "Email to verify"
//	@Success      200   {object}  dto.MessageResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/request-email-verification [post]
func (h *AuthHandler) RequestEmailVerification(w http.ResponseWriter, r *http.Request) {
	var req dto.RequestEmailVerificationRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.RequestEmailVerification(r.Context(), req.Email); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteMessage(w, r, "A verification code has been sent to your email.")
}

// Register godoc
//
//	@Summary      Register new user
//	@Description  Create a new CashPilotX account with phone number and PIN.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.RegisterRequest  true  "Registration payload"
//	@Success      201   {object}  dto.RegisterResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      409   {object}  dto.ErrorResponse
//	@Failure      422   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	ipAddress := h.ClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	resp, err := h.svc.Register(r.Context(), &req, ipAddress, userAgent)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// Login godoc
//
//	@Summary      Login
//	@Description  Authenticate with phone and PIN; returns access and refresh tokens.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.LoginRequest  true  "Login payload"
//	@Success      200   {object}  dto.LoginResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      423   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	ipAddress := h.ClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	resp, err := h.svc.Login(r.Context(), &req, ipAddress, userAgent)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// RefreshToken godoc
//
//	@Summary      Refresh access token
//	@Description  Exchange a valid refresh token for a new access/refresh token pair.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.RefreshTokenRequest  true  "Refresh token payload"
//	@Success      200   {object}  dto.RefreshTokenResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/refresh [post]
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshTokenRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	ipAddress := h.ClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	resp, err := h.svc.RefreshToken(r.Context(), &req, ipAddress, userAgent)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// Logout godoc
//
//	@Summary      Logout
//	@Description  Revoke the current session or all sessions. Requires Bearer token.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body  dto.LogoutRequest  true  "Logout options"
//	@Success      204   "No Content"
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/logout [post]
//	@Security     BearerAuth
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req dto.LogoutRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	accessToken := h.BearerToken(r)
	if accessToken == "" {
		h.WriteNoContent(w)
		return
	}

	if err := h.svc.Logout(r.Context(), accessToken, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// GetSessions godoc
//
//	@Summary      List active sessions
//	@Description  Return all active sessions for the authenticated user.
//	@Tags         auth
//	@Produce      json
//	@Success      200  {array}   dto.SessionResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/sessions [get]
//	@Security     BearerAuth
func (h *AuthHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	sessions, err := h.svc.GetUserSessions(r.Context(), userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, sessions)
}

// ForgotPIN godoc
//
//	@Summary      Request PIN reset OTP
//	@Description  Send a 6-digit OTP to the user's registered email. Accepts phone or email as identifier.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.ForgotPINRequest  true  "Phone or email"
//	@Success      200   {object}  dto.MessageResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/forgot-pin [post]
func (h *AuthHandler) ForgotPIN(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPINRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.ForgotPIN(r.Context(), &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteMessage(w, r, "If an account exists for that identifier, an OTP has been sent to the registered email.")
}

// ResetPIN godoc
//
//	@Summary      Reset PIN using OTP
//	@Description  Verify the OTP and set a new PIN. All existing sessions are revoked on success.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.ResetPINRequest  true  "OTP + new PIN"
//	@Success      200   {object}  dto.MessageResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/reset-pin [post]
func (h *AuthHandler) ResetPIN(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPINRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.ResetPIN(r.Context(), &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteMessage(w, r, "PIN reset successful. Please log in with your new PIN.")
}

// RevokeSession godoc
//
//	@Summary      Revoke a session
//	@Description  Revoke a specific session by ID for the authenticated user.
//	@Tags         auth
//	@Produce      json
//	@Param        session_id  path  string  true  "Session UUID"
//	@Success      204  "No Content"
//	@Failure      400  {object}  dto.ErrorResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/sessions/{session_id} [delete]
//	@Security     BearerAuth
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	sessionID, ok := h.MustUUIDParam(w, r, "session_id")
	if !ok {
		return
	}

	if err := h.svc.RevokeSession(r.Context(), userID, sessionID.String()); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// GetMe godoc
//
//	@Summary      Get current user
//	@Description  Returns the authenticated user's profile, including their preferred home currency.
//	@Tags         auth
//	@Produce      json
//	@Success      200  {object}  dto.UserResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/me [get]
//	@Security     BearerAuth
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.GetMe(r.Context(), userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// DeleteAccount godoc
//
//	@Summary      Delete account
//	@Description  Permanently deletes the authenticated user's account and all associated data (customers, invoices, sessions). This action is irreversible.
//	@Tags         auth
//	@Produce      json
//	@Success      204
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/me [delete]
//	@Security     BearerAuth
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteUser(r.Context(), userID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// ChangePIN godoc
//
//	@Summary      Change PIN
//	@Description  Verify the current PIN and set a new one. Clears force_pin_change and returns a fresh access token.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.ChangePINRequest  true  "Current and new PIN"
//	@Success      200   {object}  dto.ChangePINResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      422   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/change-pin [post]
//	@Security     BearerAuth
func (h *AuthHandler) ChangePIN(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req dto.ChangePINRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.ChangePIN(r.Context(), userID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// UpdateUserSettings godoc
//
//	@Summary      Update user settings
//	@Description  Updates the authenticated user's settings. Currently supports home_currency (THB, MMK, CNY).
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.UpdateUserSettingsRequest  true  "Settings payload"
//	@Success      200   {object}  dto.UserResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      422   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/auth/settings [patch]
//	@Security     BearerAuth
func (h *AuthHandler) UpdateUserSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req dto.UpdateUserSettingsRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.UpdateHomeCurrency(r.Context(), userID, req.HomeCurrency)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
