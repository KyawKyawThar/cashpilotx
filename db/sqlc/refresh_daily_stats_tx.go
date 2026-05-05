package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ── RefreshDailyStatsTx ───────────────────────────────────────────────────────
// Atomically recomputes daily_stats for a given user+date directly from the
// payments, invoices, and messages tables (no manual upsert with pre-computed
// values). Call this after RecordPaymentTx, after sending a message, or from
// a nightly background job to keep stats consistent.

type RefreshDailyStatsTxParams struct {
	UserID uuid.UUID
	Date   time.Time
}

type RefreshDailyStatsTxResult struct {
	Stats DailyStat
}

func (s *SQLStore) RefreshDailyStatsTx(ctx context.Context, arg RefreshDailyStatsTxParams) (RefreshDailyStatsTxResult, error) {
	var result RefreshDailyStatsTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		p, err := computeDailyStats(ctx, q, arg.UserID, arg.Date)
		if err != nil {
			return err
		}
		result.Stats, err = q.UpsertDailyStats(ctx, p)
		return err
	})

	return result, err
}

// computeDailyStats queries source tables inside an open transaction to
// produce a fully-derived UpsertDailyStatsParams — no caller arithmetic needed.
func computeDailyStats(ctx context.Context, q *Queries, userID uuid.UUID, date time.Time) (UpsertDailyStatsParams, error) {
	var p UpsertDailyStatsParams
	p.UserID = userID
	// Normalize to UTC midnight so the date is unambiguous regardless of
	// whether pgx sends the value as timestamptz or timestamp-without-tz.
	utc := date.UTC()
	p.Date = time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)

	// Current outstanding balance across all active invoices (global snapshot).
	const sqlOutstanding = `
		SELECT COALESCE(SUM(amount - COALESCE(paid_amount, 0)), 0)::BIGINT
		FROM invoices
		WHERE user_id = $1
		  AND status NOT IN ('paid', 'written_off')`

	if err := q.db.QueryRow(ctx, sqlOutstanding, userID).Scan(&p.TotalOutstanding); err != nil {
		return p, err
	}

	// Amount collected on this specific date.
	const sqlCollected = `
		SELECT COALESCE(SUM(amount), 0)::BIGINT
		FROM payments
		WHERE user_id = $1
		  AND created_at::date = $2::date`

	if err := q.db.QueryRow(ctx, sqlCollected, userID, p.Date).Scan(&p.TotalCollected); err != nil {
		return p, err
	}

	// Count of invoices that are still open (global snapshot).
	const sqlUnpaid = `
		SELECT COUNT(*)::int
		FROM invoices
		WHERE user_id = $1
		  AND status NOT IN ('paid', 'written_off')`

	if err := q.db.QueryRow(ctx, sqlUnpaid, userID).Scan(&p.InvoicesUnpaid); err != nil {
		return p, err
	}

	// Count of invoices marked paid on this specific date.
	const sqlPaid = `
		SELECT COUNT(*)::int
		FROM invoices
		WHERE user_id = $1
		  AND status = 'paid'
		  AND paid_at::date = $2::date`

	if err := q.db.QueryRow(ctx, sqlPaid, userID, p.Date).Scan(&p.InvoicesPaid); err != nil {
		return p, err
	}

	// Messages successfully sent on this specific date.
	const sqlMessages = `
		SELECT COUNT(*)::int
		FROM messages
		WHERE user_id = $1
		  AND status = 'sent'
		  AND sent_at::date = $2::date`

	if err := q.db.QueryRow(ctx, sqlMessages, userID, p.Date).Scan(&p.MessagesSent); err != nil {
		return p, err
	}

	return p, nil
}
