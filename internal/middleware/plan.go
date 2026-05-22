package middleware

import (
	"cashpilotx/internal/api"
	"net/http"
)

// RequirePlan returns a middleware that allows only requests whose JWT plan
// matches one of the provided plan names.
// Returns 402 Payment Required with {"upgrade":true} so the frontend can
// show the upgrade prompt instead of a generic error.
func RequirePlan(plans ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(plans))
	for _, p := range plans {
		allowed[p] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := GetPayload(r.Context())
			if payload == nil {
				api.WriteErrorWithContext(w, r, api.ErrUnauthorized(""))
				return
			}
			if payload.Role != "super_admin" {
				if _, ok := allowed[payload.Plan]; !ok {
					api.WriteErrorWithContext(w, r, api.ErrUpgradeRequired())
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
