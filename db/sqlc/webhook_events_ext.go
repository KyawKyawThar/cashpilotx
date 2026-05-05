package db

import (
	"context"

	"github.com/google/uuid"
)

// MarkWebhookProcessedImmediate marks a webhook event as processed from any
// status. Used by ProcessWebhookTx which handles the event synchronously and
// never goes through the async queue "processing" state.
func (q *Queries) MarkWebhookProcessedImmediate(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE webhook_events
		SET status = 'processed',
		    processed_at = now()
		WHERE id = $1
	`, id)
	return err
}
