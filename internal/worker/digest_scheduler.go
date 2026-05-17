package worker

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/email"
	"cashpilotx/internal/notify"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// DigestScheduler fires once per day at a configured UTC hour.
// It sends each active user (who has an email) a summary of their overdue
// and due-today invoices, plus a Web Push notification if they have a subscription.
type DigestScheduler struct {
	store   db.Store
	emailer email.Sender
	pusher  *notify.Sender // may be nil if VAPID keys are not configured
	hourUTC int
	appURL  string
}

// NewDigestScheduler creates a scheduler. pusher may be nil; email is always sent.
func NewDigestScheduler(store db.Store, emailer email.Sender, pusher *notify.Sender, hourUTC int, appURL string) *DigestScheduler {
	return &DigestScheduler{
		store:   store,
		emailer: emailer,
		pusher:  pusher,
		hourUTC: hourUTC,
		appURL:  appURL,
	}
}

// Run blocks until ctx is canceled, firing the digest at hourUTC each day.
func (d *DigestScheduler) Run(ctx context.Context) {
	log.Info().Int("hour_utc", d.hourUTC).Msg("digest scheduler started")
	for {
		next := d.nextFire()
		select {
		case <-ctx.Done():
			log.Info().Msg("digest scheduler stopping")
			return
		case <-time.After(time.Until(next)):
			d.runDigest(ctx)
		}
	}
}

// nextFire returns the next wall-clock moment for hourUTC today or tomorrow.
func (d *DigestScheduler) nextFire() time.Time {
	now := time.Now().UTC()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), d.hourUTC, 0, 0, 0, time.UTC)
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

func (d *DigestScheduler) runDigest(ctx context.Context) {
	log.Info().Msg("digest scheduler: starting daily digest run")

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	users, err := d.store.ListUsersForDigest(qCtx)
	if err != nil {
		log.Error().Err(err).Msg("digest scheduler: list users failed")
		return
	}

	log.Info().Int("user_count", len(users)).Msg("digest scheduler: processing users")

	for _, u := range users {
		if u.Email == nil || *u.Email == "" {
			continue
		}
		d.processUser(ctx, u)
	}
}

func (d *DigestScheduler) processUser(ctx context.Context, u db.ListUsersForDigestRow) {
	qCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	overdue, err := d.store.ListOverdueInvoices(qCtx, db.ListOverdueInvoicesParams{
		UserID: u.ID,
		Limit:  200,
		Offset: 0,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", u.ID.String()).Msg("digest: list overdue failed")
		return
	}

	dueToday, err := d.store.ListInvoicesDueToday(qCtx, u.ID)
	if err != nil {
		log.Error().Err(err).Str("user_id", u.ID.String()).Msg("digest: list due today failed")
		return
	}

	if len(overdue) == 0 && len(dueToday) == 0 {
		return // nothing to report
	}

	var overdueTotal int64
	for _, inv := range overdue {
		overdueTotal += int64(inv.Amount - inv.PaidAmount)
	}
	var dueTodayTotal int64
	for _, inv := range dueToday {
		dueTodayTotal += int64(inv.Amount - inv.PaidAmount)
	}

	name := u.Name
	currency := u.HomeCurrency

	data := email.DigestData{
		Name:          name,
		Currency:      currency,
		OverdueCount:  len(overdue),
		OverdueTotal:  overdueTotal,
		DueTodayCount: len(dueToday),
		DueTodayTotal: dueTodayTotal,
		AppURL:        d.appURL,
	}

	if err := d.emailer.SendDigest(qCtx, *u.Email, data); err != nil {
		log.Error().Err(err).Str("user_id", u.ID.String()).Msg("digest: send email failed")
	} else {
		log.Info().Str("user_id", u.ID.String()).Int("overdue", len(overdue)).Msg("digest: email sent")
	}

	d.sendPush(ctx, u, len(overdue), len(dueToday))
}

func (d *DigestScheduler) sendPush(ctx context.Context, u db.ListUsersForDigestRow, overdueCount, dueTodayCount int) {
	if d.pusher == nil {
		return
	}

	qCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	subs, err := d.store.ListPushSubscriptionsForUser(qCtx, u.ID)
	if err != nil || len(subs) == 0 {
		return
	}

	body := buildPushBody(overdueCount, dueTodayCount)
	payload := notify.PushPayload{
		Title: "CashPilotX — Daily Summary",
		Body:  body,
		URL:   d.appURL,
	}

	for _, sub := range subs {
		err := d.pusher.Send(ctx, notify.Subscription{
			Endpoint: sub.Endpoint,
			P256DH:   sub.P256dh,
			Auth:     sub.Auth,
		}, payload)
		if err != nil {
			if errors.Is(err, notify.ErrSubscriptionGone) {
				// Subscription expired — clean it up silently
				if deleteErr := d.store.DeletePushSubscription(ctx, db.DeletePushSubscriptionParams{
					UserID:   u.ID,
					Endpoint: sub.Endpoint,
				}); deleteErr != nil {
					log.Warn().Err(deleteErr).Str("user_id", u.ID.String()).Msg("digest: failed to delete push subscription")
				}
			} else {
				log.Warn().Err(err).Str("user_id", u.ID.String()).Msg("digest: push failed")
			}
		}
	}
}

func buildPushBody(overdueCount, dueTodayCount int) string {
	switch {
	case overdueCount > 0 && dueTodayCount > 0:
		return fmt.Sprintf("%d overdue + %d due today. Tap to review.", overdueCount, dueTodayCount)
	case overdueCount > 0:
		return fmt.Sprintf("%d invoice%s overdue. Tap to review.", overdueCount, digestPluralS(overdueCount))
	default:
		return fmt.Sprintf("%d invoice%s due today. Tap to review.", dueTodayCount, digestPluralS(dueTodayCount))
	}
}

func digestPluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
