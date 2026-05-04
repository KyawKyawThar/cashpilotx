package db

import "context"

// ProcessWebhookTx handles payment-confirmation webhooks (Stripe, PromptPay).
// Atomically records the event (idempotency guard) and recomputes the invoice
// status from the payments table. The UNIQUE(provider, event_id) constraint
// on webhook_events causes the tx to fail fast on duplicate deliveries.
//
// For inbound conversational replies (LINE "I'll pay tomorrow"), use
// ProcessInboundReplyTx instead.

type ProcessWebhookTxParams struct {
	WebhookEvent CreateWebhookEventParams
	Payment      CreatePaymentParams
	CustomerID   RecordPaymentTxParams
}

type ProcessWebhookTxResult struct {
	WebhookEvent CreateWebhookEventRow
	Payment      Payment
	Invoice      Invoice
}

func (s *SQLStore) ProcessWebhookTx(ctx context.Context, arg ProcessWebhookTxParams) (ProcessWebhookTxResult, error) {
	var result ProcessWebhookTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 1. Idempotency guard — duplicate webhook fails here with unique constraint
		var err error
		result.WebhookEvent, err = q.CreateWebhookEvent(ctx, arg.WebhookEvent)
		if err != nil {
			return err
		}

		// 2. Insert payment
		result.Payment, err = q.CreatePayment(ctx, arg.Payment)
		if err != nil {
			return err
		}

		// 3. Recompute invoice status from payments table
		result.Invoice, err = q.RecomputeInvoiceStatus(ctx, RecomputeInvoiceStatusParams{
			ID:     arg.Payment.InvoiceID,
			UserID: arg.Payment.UserID,
		})
		if err != nil {
			return err
		}

		// 4. Mark webhook processed
		return q.MarkWebhookProcessed(ctx, result.WebhookEvent.ID)
	})

	return result, err
}
