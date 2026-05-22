package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// TeamHandler manages team and team-user CRUD endpoints.
type TeamHandler struct {
	*BaseHandler
	svc service.TeamServicer
}

func NewTeamHandler(svc service.TeamServicer, base *BaseHandler) *TeamHandler {
	return &TeamHandler{BaseHandler: base, svc: svc}
}

// =============================================================================
// Teams
// =============================================================================

// CreateTeam godoc
//
//	@Summary      Create team
//	@Description  Creates a new team. Admin or Super Admin only.
//	@Tags         teams
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateTeamRequest  true  "Team payload"
//	@Success      201   {object}  dto.TeamResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      403   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/teams [post]
//	@Security     BearerAuth
func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateTeamRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.CreateTeam(r.Context(), &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ListTeams godoc
//
//	@Summary      List teams
//	@Description  Returns all teams in the organization. Admin or Super Admin only.
//	@Tags         teams
//	@Produce      json
//	@Success      200  {array}   dto.TeamResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      403  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/teams [get]
//	@Security     BearerAuth
func (h *TeamHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ListTeams(r.Context())
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// =============================================================================
// Team users
// =============================================================================

// CreateTeamUser godoc
//
//	@Summary      Create team user
//	@Description  Creates a username-based account below the caller's role. New account is forced to change PIN on first login.
//	@Tags         team-users
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateTeamUserRequest  true  "User payload"
//	@Success      201   {object}  dto.TeamMemberResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      403   {object}  dto.ErrorResponse
//	@Failure      409   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users [post]
//	@Security     BearerAuth
func (h *TeamHandler) CreateTeamUser(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.CreateTeamUserRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	orgOwnerID := scope.CallerID
	if scope.OrgOwnerID != nil {
		orgOwnerID = *scope.OrgOwnerID
	}

	resp, err := h.svc.CreateTeamUser(r.Context(), scope.CallerID, scope.CallerRole, orgOwnerID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ListTeamUsers godoc
//
//	@Summary      List team users
//	@Description  Returns org members scoped by the caller's role. Team leaders see only their team.
//	@Tags         team-users
//	@Produce      json
//	@Param        active  query     bool  false  "Filter active only (default true)"
//	@Success      200     {array}   dto.TeamMemberResponse
//	@Failure      401     {object}  dto.ErrorResponse
//	@Failure      403     {object}  dto.ErrorResponse
//	@Failure      500     {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users [get]
//	@Security     BearerAuth
func (h *TeamHandler) ListTeamUsers(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	activeOnly := r.URL.Query().Get("active") != "false"

	orgOwnerID := scope.CallerID
	if scope.OrgOwnerID != nil {
		orgOwnerID = *scope.OrgOwnerID
	}

	resp, err := h.svc.ListTeamMembers(r.Context(), scope.CallerRole, scope.TeamID, orgOwnerID, activeOnly)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// UpdateTeamUser godoc
//
//	@Summary      Update team user
//	@Description  Edits name, username, phone, or team assignment of a member below the caller's role.
//	@Tags         team-users
//	@Accept       json
//	@Produce      json
//	@Param        user_id  path      string                    true  "User UUID"
//	@Param        body     body      dto.UpdateTeamUserRequest  true  "Update payload"
//	@Success      200      {object}  dto.TeamMemberResponse
//	@Failure      400      {object}  dto.ErrorResponse
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      403      {object}  dto.ErrorResponse
//	@Failure      404      {object}  dto.ErrorResponse
//	@Failure      409      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id} [patch]
//	@Security     BearerAuth
func (h *TeamHandler) UpdateTeamUser(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	memberID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	var req dto.UpdateTeamUserRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.UpdateTeamMember(r.Context(), scope.CallerRole, memberID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// DeactivateTeamUser godoc
//
//	@Summary      Deactivate team user
//	@Description  Soft-deletes a member account. The user immediately loses all access.
//	@Tags         team-users
//	@Produce      json
//	@Param        user_id  path  string  true  "User UUID"
//	@Success      204      "No Content"
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      403      {object}  dto.ErrorResponse
//	@Failure      404      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id} [delete]
//	@Security     BearerAuth
func (h *TeamHandler) DeactivateTeamUser(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	memberID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	if err := h.svc.DeactivateMember(r.Context(), scope.CallerRole, memberID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// ResetMemberPIN godoc
//
//	@Summary      Reset member PIN
//	@Description  Sets a temporary PIN for a member and forces a change on next login. Caller must be superior to the target.
//	@Tags         team-users
//	@Accept       json
//	@Produce      json
//	@Param        user_id  path  string                  true  "User UUID"
//	@Param        body     body  dto.AdminResetPINRequest  true  "New temporary PIN"
//	@Success      204      "No Content"
//	@Failure      400      {object}  dto.ErrorResponse
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      403      {object}  dto.ErrorResponse
//	@Failure      404      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id}/reset-pin [post]
//	@Security     BearerAuth
func (h *TeamHandler) ResetMemberPIN(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	memberID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	var req dto.AdminResetPINRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.ResetMemberPIN(r.Context(), scope.CallerRole, memberID, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// =============================================================================
// Permission grants (super_admin only)
// =============================================================================

// GrantPermission godoc
//
//	@Summary      Grant permission
//	@Description  Creates a per-user exception grant. Super Admin only.
//	@Tags         team-users
//	@Accept       json
//	@Produce      json
//	@Param        user_id  path      string                      true  "User UUID"
//	@Param        body     body      dto.GrantPermissionRequest   true  "Grant payload"
//	@Success      201      {object}  dto.PermissionGrantResponse
//	@Failure      400      {object}  dto.ErrorResponse
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      403      {object}  dto.ErrorResponse
//	@Failure      404      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id}/grants [post]
//	@Security     BearerAuth
func (h *TeamHandler) GrantPermission(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	targetID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	var req dto.GrantPermissionRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.GrantPermission(r.Context(), callerID, targetID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ListGrants godoc
//
//	@Summary      List permission grants
//	@Description  Returns all exception grants for a user. Super Admin only.
//	@Tags         team-users
//	@Produce      json
//	@Param        user_id  path      string  true  "User UUID"
//	@Success      200      {array}   dto.PermissionGrantResponse
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      403      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id}/grants [get]
//	@Security     BearerAuth
func (h *TeamHandler) ListGrants(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	targetID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	resp, err := h.svc.ListGrants(r.Context(), targetID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// RevokeGrant godoc
//
//	@Summary      Revoke permission grant
//	@Description  Deletes a specific exception grant. Super Admin only.
//	@Tags         team-users
//	@Produce      json
//	@Param        user_id   path  string  true  "User UUID"
//	@Param        grant_id  path  string  true  "Grant UUID"
//	@Success      204       "No Content"
//	@Failure      401       {object}  dto.ErrorResponse
//	@Failure      403       {object}  dto.ErrorResponse
//	@Failure      404       {object}  dto.ErrorResponse
//	@Failure      500       {object}  dto.ErrorResponse
//	@Router       /api/v1/team/users/{user_id}/grants/{grant_id} [delete]
//	@Security     BearerAuth
func (h *TeamHandler) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	targetID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	grantID, ok := h.MustUUIDParam(w, r, "grant_id")
	if !ok {
		return
	}

	if err := h.svc.RevokeGrant(r.Context(), targetID, grantID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}
