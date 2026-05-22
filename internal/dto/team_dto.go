package dto

import (
	db "cashpilotx/db/sqlc"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Teams
// =============================================================================

type CreateTeamRequest struct {
	Name    string     `json:"name" validate:"required,min=1,max=100"`
	AdminID *uuid.UUID `json:"admin_id"`
}

type UpdateTeamRequest struct {
	Name    *string    `json:"name" validate:"omitempty,min=1,max=100"`
	AdminID *uuid.UUID `json:"admin_id"`
}

type TeamResponse struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	AdminID   *uuid.UUID `json:"admin_id"`
	CreatedAt time.Time  `json:"created_at"`
}

func ToTeamResponse(t db.Team) *TeamResponse {
	return &TeamResponse{
		ID:        t.ID,
		Name:      t.Name,
		AdminID:   t.AdminID,
		CreatedAt: t.CreatedAt,
	}
}

// =============================================================================
// Team users
// =============================================================================

type CreateTeamUserRequest struct {
	Name     string     `json:"name" validate:"required,min=1,max=100"`
	Username string     `json:"username" validate:"required,min=3,max=50,alphanum"`
	Phone    *string    `json:"phone"`
	PIN      string     `json:"pin" validate:"required,len=6,numeric"`
	Role     string     `json:"role" validate:"required,oneof=admin accountant cs_team sales"`
	TeamID   *uuid.UUID `json:"team_id"`
}

type UpdateTeamUserRequest struct {
	Name     *string    `json:"name" validate:"omitempty,min=1,max=100"`
	Username *string    `json:"username" validate:"omitempty,min=3,max=50,alphanum"`
	Phone    *string    `json:"phone"`
	Role     *string    `json:"role" validate:"omitempty,oneof=admin accountant cs_team sales"`
	TeamID   *uuid.UUID `json:"team_id"`
}

type AdminResetPINRequest struct {
	NewPIN string `json:"new_pin" validate:"required,len=6,numeric"`
}

type TeamMemberResponse struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Username       *string    `json:"username"`
	Phone          *string    `json:"phone,omitempty"`
	Role           string     `json:"role"`
	TeamID         *uuid.UUID `json:"team_id"`
	IsActive       bool       `json:"is_active"`
	ForcePinChange bool       `json:"force_pin_change"`
	CreatedBy      *uuid.UUID `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

func ToTeamMemberResponse(u db.User) *TeamMemberResponse {
	return &TeamMemberResponse{
		ID:             u.ID,
		Name:           u.Name,
		Username:       u.Username,
		Phone:          u.Phone,
		Role:           string(u.Role),
		TeamID:         u.TeamID,
		IsActive:       u.IsActive,
		ForcePinChange: u.ForcePinChange,
		CreatedBy:      u.CreatedBy,
		CreatedAt:      u.CreatedAt,
	}
}

// =============================================================================
// Permission grants
// =============================================================================

type GrantPermissionRequest struct {
	Resource  string     `json:"resource" validate:"required"`
	Action    string     `json:"action" validate:"required"`
	Scope     string     `json:"scope" validate:"required,oneof=global team own"`
	ScopeTeam *uuid.UUID `json:"scope_team_id"`
	Reason    *string    `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type PermissionGrantResponse struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	PermissionID uuid.UUID  `json:"permission_id"`
	Resource     string     `json:"resource"`
	Action       string     `json:"action"`
	Scope        string     `json:"scope"`
	ScopeTeamID  *uuid.UUID `json:"scope_team_id"`
	GrantedBy    uuid.UUID  `json:"granted_by"`
	Reason       *string    `json:"reason"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func ToPermissionGrantResponse(g db.UserPermissionGrant, p db.Permission) *PermissionGrantResponse {
	return &PermissionGrantResponse{
		ID:           g.ID,
		UserID:       g.UserID,
		PermissionID: g.PermissionID,
		Resource:     p.Resource,
		Action:       p.Action,
		Scope:        string(g.Scope),
		ScopeTeamID:  g.ScopeTeamID,
		GrantedBy:    g.GrantedBy,
		Reason:       g.Reason,
		ExpiresAt:    g.ExpiresAt,
		CreatedAt:    g.CreatedAt,
	}
}

func PermissionGrantRowToResponse(row db.ListPermissionGrantsWithPermissionRow) *PermissionGrantResponse {
	return &PermissionGrantResponse{
		ID:           row.ID,
		UserID:       row.UserID,
		PermissionID: row.PermissionID,
		Resource:     row.Resource,
		Action:       row.Action,
		Scope:        string(row.Scope),
		ScopeTeamID:  row.ScopeTeamID,
		GrantedBy:    row.GrantedBy,
		Reason:       row.Reason,
		ExpiresAt:    row.ExpiresAt,
		CreatedAt:    row.CreatedAt,
	}
}
