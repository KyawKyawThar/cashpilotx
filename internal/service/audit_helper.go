package service

import (
	db "cashpilotx/db/sqlc"
	"context"
	"time"
)

// logAuditAsync writes an audit log entry in a detached goroutine so it never
// blocks the caller or fails the originating request if the insert errors.
func logAuditAsync(ctx context.Context, store db.Store, params db.CreateOperatorAuditLogParams) {
	go func() {
		aCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = store.CreateOperatorAuditLog(aCtx, params) //nolint:errcheck // fire-and-forget audit log
	}()
}
