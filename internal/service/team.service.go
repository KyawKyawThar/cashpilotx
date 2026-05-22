package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/utils"
	"context"

	"github.com/google/uuid"
)

// roleRank returns a numeric rank so we can compare role seniority.
// Higher = more powerful.
func roleRank(r db.UserRole) int {
	switch r {
	case db.UserRoleSuperAdmin:
		return 5
	case db.UserRoleAdmin:
		return 4
	case db.UserRoleAccountant:
		return 3
	case db.UserRoleCsTeam:
		return 2
	case db.UserRoleSales:
		return 1
	}
	return 0
}

// =============================================================================
// Teams
// =============================================================================

func (s *TeamService) CreateTeam(
	ctx context.Context,
	req *dto.CreateTeamRequest,
) (*dto.TeamResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	team, err := s.store.CreateTeam(dbCtx, db.CreateTeamParams{
		Name:    req.Name,
		AdminID: req.AdminID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToTeamResponse(team), nil
}

func (s *TeamService) ListTeams(
	ctx context.Context,
) ([]*dto.TeamResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListTeams(dbCtx)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.TeamResponse, 0, len(rows))
	for _, t := range rows {
		out = append(out, dto.ToTeamResponse(t))
	}
	return out, nil
}

// =============================================================================
// Team users
// =============================================================================

// CreateTeamUser creates a username-based account below the caller's role.
// The new account is forced to change PIN on first login.
func (s *TeamService) CreateTeamUser(
	ctx context.Context,
	callerID uuid.UUID,
	callerRole db.UserRole,
	orgOwnerID uuid.UUID,
	req *dto.CreateTeamUserRequest,
) (*dto.TeamMemberResponse, error) {
	targetRole := db.UserRole(req.Role)
	if roleRank(callerRole) <= roleRank(targetRole) {
		return nil, api.ErrForbidden("you can only create roles strictly below your own")
	}

	pinHash, err := utils.HashPassword(req.PIN)
	if err != nil {
		return nil, api.ErrInternal(err)
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	orgOwner, err := s.store.GetUserByID(dbCtx, orgOwnerID)
	if err != nil {
		return nil, api.ErrInternal(err)
	}

	user, err := s.store.CreateTeamUser(dbCtx, db.CreateTeamUserParams{
		Name:         req.Name,
		Username:     &req.Username,
		PinHash:      pinHash,
		HomeCurrency: "MMK",
		Plan:         "pro",
		Role:         targetRole,
		TeamID:       req.TeamID,
		OrgOwnerID:   &orgOwnerID,
		CreatedBy:    &callerID,
		BusinessName: orgOwner.BusinessName,
	})
	if err != nil {
		if api.IsUniqueViolation(err) {
			return nil, api.ErrDuplicateResource("User", "username")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToTeamMemberResponse(user), nil
}

// ListTeamMembers returns org members scoped by the caller's role.
// cs_team sees only their team; admin/super_admin see all.
func (s *TeamService) ListTeamMembers(
	ctx context.Context,
	callerRole db.UserRole,
	callerTeamID *uuid.UUID,
	orgOwnerID uuid.UUID,
	activeOnly bool,
) ([]*dto.TeamMemberResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var rows []db.User
	var err error

	if activeOnly {
		rows, err = s.store.ListActiveOrgUsers(dbCtx, &orgOwnerID)
	} else {
		rows, err = s.store.ListAllOrgUsers(dbCtx, &orgOwnerID)
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	// cs_team sees only members in their own team.
	if callerRole == db.UserRoleCsTeam && callerTeamID != nil {
		filtered := rows[:0]
		for _, u := range rows {
			if u.TeamID != nil && *u.TeamID == *callerTeamID {
				filtered = append(filtered, u)
			}
		}
		rows = filtered
	}

	out := make([]*dto.TeamMemberResponse, 0, len(rows))
	for _, u := range rows {
		out = append(out, dto.ToTeamMemberResponse(u))
	}
	return out, nil
}

// UpdateTeamMember updates name, username, phone, team assignment, or role.
func (s *TeamService) UpdateTeamMember(
	ctx context.Context,
	callerRole db.UserRole,
	memberID uuid.UUID,
	req *dto.UpdateTeamUserRequest,
) (*dto.TeamMemberResponse, error) {
	if callerRole != db.UserRoleSuperAdmin {
		return nil, api.ErrForbidden("only super_admin can edit team accounts")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// If a role change is requested, validate the rank boundary and update separately.
	if req.Role != nil {
		targetRole := db.UserRole(*req.Role)
		if roleRank(callerRole) <= roleRank(targetRole) {
			return nil, api.ErrForbidden("you can only assign roles strictly below your own")
		}
		if _, err := s.store.UpdateUserRole(dbCtx, db.UpdateUserRoleParams{
			ID:   memberID,
			Role: targetRole,
		}); err != nil {
			return nil, api.FromPgError(err)
		}
	}

	updated, err := s.store.UpdateTeamUser(dbCtx, db.UpdateTeamUserParams{
		ID:       memberID,
		Name:     req.Name,
		Username: req.Username,
		Phone:    req.Phone,
		TeamID:   req.TeamID,
	})
	if err != nil {
		if api.IsUniqueViolation(err) {
			return nil, api.ErrDuplicateResource("User", "username")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToTeamMemberResponse(updated), nil
}

// DeactivateMember soft-deletes a team member (sets is_active = false).
func (s *TeamService) DeactivateMember(
	ctx context.Context,
	callerRole db.UserRole,
	memberID uuid.UUID,
) error {
	if callerRole != db.UserRoleSuperAdmin {
		return api.ErrForbidden("only super_admin can deactivate team accounts")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.store.GetUserByID(dbCtx, memberID); err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("User")
		}
		return api.FromPgError(err)
	}

	if err := s.store.DeactivateUser(dbCtx, memberID); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

// ResetMemberPIN sets a temporary PIN and forces a change on next login.
// Only super_admin may reset any member's PIN.
func (s *TeamService) ResetMemberPIN(
	ctx context.Context,
	callerRole db.UserRole,
	memberID uuid.UUID,
	req *dto.AdminResetPINRequest,
) error {
	if callerRole != db.UserRoleSuperAdmin {
		return api.ErrForbidden("only super_admin can reset member PINs")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	target, err := s.store.GetUserByID(dbCtx, memberID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("User")
		}
		return api.FromPgError(err)
	}

	if roleRank(callerRole) <= roleRank(target.Role) {
		return api.ErrForbidden("you can only reset PINs for users with roles strictly below your own")
	}

	pinHash, err := utils.HashPassword(req.NewPIN)
	if err != nil {
		return api.ErrInternal(err)
	}

	if err := s.store.SetForcePinChange(dbCtx, db.SetForcePinChangeParams{
		ID:      memberID,
		PinHash: pinHash,
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

// =============================================================================
// Permission grants (super_admin only — enforced at route level)
// =============================================================================

// GrantPermission creates a per-user exception grant.
func (s *TeamService) GrantPermission(
	ctx context.Context,
	callerID uuid.UUID,
	targetUserID uuid.UUID,
	req *dto.GrantPermissionRequest,
) (*dto.PermissionGrantResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	perm, err := s.store.GetPermissionByResourceAction(dbCtx, db.GetPermissionByResourceActionParams{
		Resource: req.Resource,
		Action:   req.Action,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("Permission")
		}
		return nil, api.FromPgError(err)
	}

	grant, err := s.store.CreatePermissionGrant(dbCtx, db.CreatePermissionGrantParams{
		UserID:       targetUserID,
		PermissionID: perm.ID,
		Scope:        db.DataScope(req.Scope),
		ScopeTeamID:  req.ScopeTeam,
		GrantedBy:    callerID,
		Reason:       req.Reason,
		ExpiresAt:    req.ExpiresAt,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToPermissionGrantResponse(grant, perm), nil
}

// ListGrants returns all permission grants for a user, including resource and action details.
func (s *TeamService) ListGrants(
	ctx context.Context,
	targetUserID uuid.UUID,
) ([]*dto.PermissionGrantResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListPermissionGrantsWithPermission(dbCtx, targetUserID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.PermissionGrantResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.PermissionGrantRowToResponse(row))
	}
	return out, nil
}

// RevokeGrant deletes a specific grant.
func (s *TeamService) RevokeGrant(
	ctx context.Context,
	targetUserID uuid.UUID,
	grantID uuid.UUID,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.store.DeletePermissionGrant(dbCtx, db.DeletePermissionGrantParams{
		ID:     grantID,
		UserID: targetUserID,
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}
