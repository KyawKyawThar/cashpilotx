package middleware

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"net/http"
)

// permKey is "resource:action".
type permKey = string

// rolePermMap mirrors the seed in 000032 + 000033 migrations.
// Maps role → set of "resource:action" keys that role is allowed to perform.
var rolePermMap = map[db.UserRole]map[permKey]struct{}{

	// super_admin — full access to everything including creating admins
	db.UserRoleSuperAdmin: buildPermSet(
		"invoice:view", "invoice:view_history", "invoice:create", "invoice:edit", "invoice:delete", "invoice:export",
		"customer:view", "customer:create", "customer:edit", "customer:delete", "customer:view_risk_profile", "customer:set_credit_limit",
		"payment:view", "payment:record", "payment:void",
		"reminder:view", "reminder:create", "reminder:cancel",
		"message:view", "message:send",
		"audit_log:view", "audit_log:revert",
		"report:view", "report:export",
		"recovery_queue:view", "recovery_queue:action",
		"product:view_selling", "product:view_cost", "product:manage",
		"channel_config:view", "channel_config:manage",
		"delivery_service:view", "delivery_service:manage",
		"team:view", "team:create_admin", "team:create_accountant", "team:create_cs_team", "team:create_sales",
		"team:edit", "team:delete", "team:reset_pin", "team:grant_permission",
	),

	// admin — full access except creating other admins and granting exceptions
	db.UserRoleAdmin: buildPermSet(
		"invoice:view", "invoice:view_history", "invoice:create", "invoice:edit", "invoice:delete", "invoice:export",
		"customer:view", "customer:create", "customer:edit", "customer:delete", "customer:view_risk_profile", "customer:set_credit_limit",
		"payment:view", "payment:record", "payment:void",
		"reminder:view", "reminder:create", "reminder:cancel",
		"message:view", "message:send",
		"audit_log:view", "audit_log:revert",
		"report:view", "report:export",
		"recovery_queue:view", "recovery_queue:action",
		"product:view_selling", "product:view_cost", "product:manage",
		"channel_config:view", "channel_config:manage",
		"delivery_service:view", "delivery_service:manage",
		"team:view", "team:create_accountant", "team:create_cs_team", "team:create_sales",
		"team:edit", "team:delete",
	),

	// accountant — full access except creating CS Team accounts, creating customers, and granting exceptions
	db.UserRoleAccountant: buildPermSet(
		"invoice:view", "invoice:view_history", "invoice:create", "invoice:edit", "invoice:delete", "invoice:export",
		"customer:view", "customer:edit", "customer:delete", "customer:view_risk_profile", "customer:set_credit_limit",
		"payment:view", "payment:record", "payment:void",
		"reminder:view", "reminder:create", "reminder:cancel",
		"message:view", "message:send",
		"audit_log:view", "audit_log:revert",
		"report:view", "report:export",
		"recovery_queue:view", "recovery_queue:action",
		"product:view_selling", "product:view_cost",
		"channel_config:view",
		"delivery_service:view", "delivery_service:manage",
		"team:view", "team:create_accountant", "team:create_sales",
		"team:edit",
	),

	// cs_team — all except payments, recovery queue, cost price, manage actions
	db.UserRoleCsTeam: buildPermSet(
		"invoice:view", "invoice:view_history", "invoice:create", "invoice:edit", "invoice:delete", "invoice:export",
		"customer:view", "customer:create", "customer:edit", "customer:delete", "customer:view_risk_profile", "customer:set_credit_limit",
		"payment:view",
		"reminder:view", "reminder:create", "reminder:cancel",
		"message:view", "message:send",
		"audit_log:view",
		"report:view", "report:export",
		"product:view_selling",
		"channel_config:view",
		"delivery_service:view",
		"team:view",
	),

	// sales — can only create and view customers (own scope) + own report row
	db.UserRoleSales: buildPermSet(
		"customer:view", "customer:create",
		"product:view_selling",
		"delivery_service:view",
		"report:view",
	),
}

func buildPermSet(keys ...string) map[permKey]struct{} {
	m := make(map[permKey]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

// RequirePermission returns a middleware that enforces the role-permission map.
// resource and action must match the keys in rolePermMap (e.g. "invoice", "view").
func RequirePermission(resource, action string) func(http.Handler) http.Handler {
	key := resource + ":" + action

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := GetPayload(r.Context())
			if payload == nil {
				api.WriteErrorWithContext(w, r, api.ErrUnauthorized(""))
				return
			}

			role := db.UserRole(payload.Role)
			perms, roleKnown := rolePermMap[role]
			if !roleKnown {
				api.WriteErrorWithContext(w, r, api.ErrForbidden(""))
				return
			}
			if _, allowed := perms[key]; !allowed {
				api.WriteErrorWithContext(w, r, api.ErrForbidden(""))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
