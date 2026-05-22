package middleware

import (
	db "cashpilotx/db/sqlc"
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ScopeInfo is computed once per request from the JWT payload and stored in
// context by ScopeContext. Handlers read it to decide effective query params.
type ScopeInfo struct {
	// Scope mirrors the role_permissions scope for this role.
	Scope db.DataScope
	// CallerID is the authenticated user's own ID.
	CallerID uuid.UUID
	// CallerRole is the authenticated user's role.
	CallerRole db.UserRole
	// EffectiveUserID is the org owner's ID — the user_id stored on every
	// data row (customers, invoices, etc.). For super_admin this equals
	// CallerID; for team accounts it equals the super_admin's org_owner_id.
	EffectiveUserID uuid.UUID
	// TeamID is non-nil for cs_team / sales.
	TeamID *uuid.UUID
	// OrgOwnerID mirrors payload.OrgOwnerID — the super_admin's user ID.
	OrgOwnerID *uuid.UUID
}

const contextKeyScope contextKey = "rbac_scope"

// ScopeContext resolves ScopeInfo from the JWT payload already in context and
// stores it for downstream handlers. Must run after JWTAuthWithService.
func ScopeContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := GetPayload(r.Context())
		if payload == nil {
			next.ServeHTTP(w, r)
			return
		}

		role := db.UserRole(payload.Role)

		effectiveUserID := payload.UserID
		if payload.OrgOwnerID != nil {
			effectiveUserID = *payload.OrgOwnerID
		}

		scope := db.DataScopeGlobal
		switch role {
		case db.UserRoleSales:
			scope = db.DataScopeOwn
		case db.UserRoleSuperAdmin, db.UserRoleAdmin, db.UserRoleAccountant, db.UserRoleCsTeam:
			// global scope — sees all org data
		}

		info := &ScopeInfo{
			Scope:           scope,
			CallerID:        payload.UserID,
			CallerRole:      role,
			EffectiveUserID: effectiveUserID,
			TeamID:          payload.TeamID,
			OrgOwnerID:      payload.OrgOwnerID,
		}

		ctx := context.WithValue(r.Context(), contextKeyScope, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetScope returns the ScopeInfo for the current request, or a safe default
// (global scope, caller = EffectiveUserID) if not set.
func GetScope(ctx context.Context) *ScopeInfo {
	if s, ok := ctx.Value(contextKeyScope).(*ScopeInfo); ok && s != nil {
		return s
	}
	return nil
}
