// Package worker implements the AI Draft Worker that reads jobs from the
// fl:generate Redis stream, builds recovery message drafts, writes them to the
// database, and notifies the SSE handler via Pub/Sub.
package worker

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/cache"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	consumerGroup = "draft-workers"
	maxRetries    = 3
	blockDuration = 5 * time.Second
)

// DraftWorker reads from the fl:generate stream and writes AI draft messages.
type DraftWorker struct {
	store    db.Store
	redis    *cache.RedisClient
	consumer string // unique per process; prevents competing consumers from sharing messages
}

// New creates a DraftWorker. consumer should be unique per process instance
// (e.g. hostname + pid).
func New(store db.Store, redis *cache.RedisClient, consumer string) *DraftWorker {
	return &DraftWorker{
		store:    store,
		redis:    redis,
		consumer: consumer,
	}
}

// Run bootstraps the consumer group and processes jobs until ctx is canceled.
func (w *DraftWorker) Run(ctx context.Context) error {
	if err := w.redis.EnsureConsumerGroup(ctx, consumerGroup); err != nil {
		return fmt.Errorf("worker: ensure consumer group: %w", err)
	}
	log.Info().Str("consumer", w.consumer).Msg("draft worker started")

	// First, re-deliver any unacked messages that survived a previous crash.
	w.processPending(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("draft worker shutting down")
			return nil
		default:
		}

		payloads, ids, err := w.redis.ReadGenerateStream(ctx, consumerGroup, w.consumer, 10, blockDuration, ">")
		if err != nil {
			log.Error().Err(err).Msg("draft worker: read stream error")
			continue
		}

		for i, p := range payloads {
			w.processJob(ctx, p, ids[i])
		}
	}
}

// processPending re-delivers messages that were claimed but never acked
// (e.g. after a crash). Reads from "0" to get pending entries.
func (w *DraftWorker) processPending(ctx context.Context) {
	payloads, ids, err := w.redis.ReadGenerateStream(ctx, consumerGroup, w.consumer, 100, 0, "0")
	if err != nil {
		log.Error().Err(err).Msg("draft worker: re-deliver pending messages failed")
		return
	}
	for i, p := range payloads {
		w.processJob(ctx, p, ids[i])
	}
}

// processJob handles a single draft-generation job with retry + backoff.
func (w *DraftWorker) processJob(ctx context.Context, p cache.GeneratePayload, streamID string) {
	log.Info().
		Str("invoice_id", p.InvoiceID).
		Str("user_id", p.UserID).
		Str("stream_id", streamID).
		Msg("processing draft job")

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := w.generate(ctx, p); err != nil {
			lastErr = err
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("invoice_id", p.InvoiceID).
				Msg("draft generation failed, retrying")
			if attempt < maxRetries {
				backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
			}
			continue
		}
		// Success — ack the stream entry.
		if ackErr := w.redis.AckGenerateStream(ctx, consumerGroup, streamID); ackErr != nil {
			log.Error().Err(ackErr).Str("stream_id", streamID).Msg("failed to ack stream entry")
		}
		return
	}

	// All retries exhausted — mark the message failed if we have a message ID.
	log.Error().
		Err(lastErr).
		Str("invoice_id", p.InvoiceID).
		Msg("draft generation failed after all retries")

	// Ack so the entry leaves the pending list (we've given up on it).
	if ackErr := w.redis.AckGenerateStream(ctx, consumerGroup, streamID); ackErr != nil {
		log.Error().Err(ackErr).Str("stream_id", streamID).Msg("failed to ack failed stream entry")
	}
}

// draftMeta is stored in messages.meta for AI-generated drafts.
type draftMeta struct {
	RecoveryScore int    `json:"recovery_score"`
	RiskLevel     string `json:"risk_level"`
	BestWindow    string `json:"best_window,omitempty"`
}

// generate loads data, builds a draft, writes it to the DB, and notifies SSE.
func (w *DraftWorker) generate(ctx context.Context, p cache.GeneratePayload) error {
	invoiceID, err := uuid.Parse(p.InvoiceID)
	if err != nil {
		return fmt.Errorf("invalid invoice_id %q: %w", p.InvoiceID, err)
	}
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id %q: %w", p.UserID, err)
	}
	customerID, err := uuid.Parse(p.CustomerID)
	if err != nil {
		return fmt.Errorf("invalid customer_id %q: %w", p.CustomerID, err)
	}

	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	inv, err := w.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("get invoice: %w", err)
	}

	customer, err := w.store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
		ID:     customerID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("get customer: %w", err)
	}

	// Risk profile is optional — continue without it if none exists yet.
	var riskScore int32
	var riskLevel, bestWindow string
	risk, riskErr := w.store.GetCustomerRiskProfile(dbCtx, db.GetCustomerRiskProfileParams{
		UserID:     userID,
		CustomerID: customerID,
	})
	if riskErr == nil {
		riskScore = risk.RiskScore
		riskLevel = risk.RiskLevel
		if risk.BestContactWindow != nil {
			bestWindow = *risk.BestContactWindow
		}
	}

	daysOverdue := int(time.Since(inv.DueDate).Hours() / 24)
	amountStr := fmt.Sprintf("%d", inv.Amount/100)
	content := buildDraftContent(customer.Name, amountStr, inv.Currency, daysOverdue, p.Tone, p.Channel)

	meta := draftMeta{
		RecoveryScore: int(riskScore),
		RiskLevel:     riskLevel,
		BestWindow:    bestWindow,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	msg, err := w.store.CreateMessage(dbCtx, db.CreateMessageParams{
		UserID:     userID,
		InvoiceID:  &invoiceID,
		CustomerID: customerID,
		Direction:  "outbound",
		Channel:    p.Channel,
		Content:    content,
		Tone:       p.Tone,
		Column8:    "draft",  // status
		Column9:    "ai",     // source
		Column10:   metaJSON, // meta
	})
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}

	if pubErr := w.redis.PublishDraftReady(ctx, p.UserID, msg.ID.String(), p.InvoiceID); pubErr != nil {
		// Non-fatal: the draft is saved; SSE notification is best-effort.
		log.Warn().Err(pubErr).Str("message_id", msg.ID.String()).Msg("failed to publish draft_ready event")
	}

	log.Info().
		Str("message_id", msg.ID.String()).
		Str("invoice_id", p.InvoiceID).
		Msg("draft message created")
	return nil
}

// buildDraftContent produces a template-based recovery message.
// Replace this function body when BYOK AI integration is added.
func buildDraftContent(customerName, amount, currency string, daysOverdue int, tone, channel string) string {
	var urgency string
	switch {
	case daysOverdue <= 0:
		urgency = "is due soon"
	case daysOverdue == 1:
		urgency = "was due yesterday"
	default:
		urgency = fmt.Sprintf("is %d days overdue", daysOverdue)
	}

	var greeting string
	switch strings.ToLower(tone) {
	case "firm":
		greeting = fmt.Sprintf("Dear %s, this is a formal notice", customerName)
	case "urgent":
		greeting = fmt.Sprintf("URGENT: %s, immediate action required", customerName)
	default: // polite
		greeting = fmt.Sprintf("Dear %s", customerName)
	}

	channelNote := ""
	if channel == "line" || channel == "whatsapp" {
		channelNote = " Please reply to this message to confirm."
	}

	return fmt.Sprintf(
		"%s, your invoice for %s %s %s. Please arrange payment at your earliest convenience.%s",
		greeting, currency, amount, urgency, channelNote,
	)
}
