package worker

import (
	db "cashpilotx/db/sqlc"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type scoreFactor struct {
	Label  string `json:"label"`
	Signal string `json:"signal"`
	Color  string `json:"color"`
}

const (
	collectionStateNoResponse = "no_response"
	timeframeToday            = "Today"
	timeframeThisWeek         = "This week"
)

// NewNaturalLanguageScorer returns a Scorer that loads full invoice + customer +
// risk-profile context and produces outcome_text in plain business language —
// no internal signal codes like "friday_pattern" or "firm_tone_effective".
//
//nolint:gocognit,gocyclo // scoring logic is inherently complex; splitting would obscure the business rules
func NewNaturalLanguageScorer(store db.Store) db.Scorer {
	return func(ctx context.Context, row db.ListUnscoredRow) (db.InvoiceScore, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		inv, err := store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
			ID:     row.ID,
			UserID: row.UserID,
		})
		if err != nil {
			return db.InvoiceScore{}, fmt.Errorf("load invoice: %w", err)
		}

		customer, err := store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
			ID:     row.CustomerID,
			UserID: row.UserID,
		})
		if err != nil {
			return db.InvoiceScore{}, fmt.Errorf("load customer: %w", err)
		}

		// Risk profile is optional — score without it if not yet computed.
		risk, _ := store.GetCustomerRiskProfile(dbCtx, db.GetCustomerRiskProfileParams{ //nolint:errcheck // optional — a missing risk profile is expected; score without it
			UserID:     row.UserID,
			CustomerID: row.CustomerID,
		})

		now := time.Now()
		daysOverdue := int(now.Sub(inv.DueDate).Hours() / 24)
		firstName := firstWord(customer.Name)
		todayName := now.Weekday().String()
		tomorrowName := now.Add(24 * time.Hour).Weekday().String()

		var factors []scoreFactor
		score := 0.5

		// ── Best contact day ─────────────────────────────────────────
		if risk.BestContactDay != nil && *risk.BestContactDay != "" {
			best := *risk.BestContactDay
			switch {
			case strings.EqualFold(best, todayName):
				score += 0.2
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Today is %s — %s's best day to respond", best, firstName),
					Signal: "high", Color: "green",
				})
			case strings.EqualFold(best, tomorrowName):
				score -= 0.1
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Tomorrow (%s) is their best day to respond — consider waiting", best),
					Signal: "med", Color: "amber",
				})
			default:
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Best day to contact: %s", best),
					Signal: "low", Color: "amber",
				})
			}
		}

		// ── Best contact window ──────────────────────────────────────
		if risk.BestContactWindow != nil && *risk.BestContactWindow != "" {
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("Responds best between %s", *risk.BestContactWindow),
				Signal: "med", Color: "green",
			})
		}

		// ── Days overdue ─────────────────────────────────────────────
		switch {
		case daysOverdue > 60:
			score += 0.2
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("%d days overdue — well past due, escalation needed", daysOverdue),
				Signal: "high", Color: "red",
			})
		case daysOverdue > 30:
			score += 0.15
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("%d days past due date — pressure is building", daysOverdue),
				Signal: "high", Color: "red",
			})
		case daysOverdue > 0:
			score += 0.1
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("%d days past due date", daysOverdue),
				Signal: "med", Color: "amber",
			})
		default:
			factors = append(factors, scoreFactor{
				Label:  "Due today or upcoming — timely nudge",
				Signal: "med", Color: "amber",
			})
		}

		// ── Follow-up silence ────────────────────────────────────────
		switch {
		case inv.FollowUpCount >= 3:
			score += 0.15
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("%d reminders sent with no reply — escalate tone or channel", inv.FollowUpCount),
				Signal: "high", Color: "red",
			})
		case inv.FollowUpCount > 0 && inv.LastContactedAt != nil:
			silentDays := int(now.Sub(*inv.LastContactedAt).Hours() / 24)
			if silentDays > 7 {
				score += 0.1
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("%d reminders sent — %d days of silence, time to follow up", inv.FollowUpCount, silentDays),
					Signal: "high", Color: "amber",
				})
			} else {
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Last message sent %d days ago — still within reply window", silentDays),
					Signal: "med", Color: "amber",
				})
			}
		default:
			factors = append(factors, scoreFactor{
				Label:  "No contact made yet — first outreach",
				Signal: "low", Color: "amber",
			})
		}

		// ── Partial payment ──────────────────────────────────────────
		if inv.PaidAmount > 0 && inv.PaidAmount < inv.Amount {
			score += 0.1
			paidPct := float64(inv.PaidAmount) / float64(inv.Amount) * 100
			factors = append(factors, scoreFactor{
				Label:  fmt.Sprintf("Already paid %.0f%% of invoice — they're following through, balance reminder is enough", paidPct),
				Signal: "med", Color: "green",
			})
		}

		// ── Historical pattern ───────────────────────────────────────
		if risk.AvgDaysToPay != nil {
			avg := *risk.AvgDaysToPay
			if avg <= 7 {
				score += 0.1
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Usually pays within %d days — this invoice is running late, worth a nudge", avg),
					Signal: "high", Color: "amber",
				})
			} else if avg > 30 {
				factors = append(factors, scoreFactor{
					Label:  fmt.Sprintf("Typically takes %d days to pay — consistent slow payer", avg),
					Signal: "med", Color: "red",
				})
			}
		}
		if risk.InvoicesOnTime > 0 && risk.InvoicesLate == 0 {
			score += 0.05
			factors = append(factors, scoreFactor{
				Label:  "Always paid on time before — this delay is out of character",
				Signal: "high", Color: "green",
			})
		}

		// ── Collection state ─────────────────────────────────────────
		switch inv.CollectionState {
		case collectionStateNoResponse:
			score += 0.1
			factors = append(factors, scoreFactor{
				Label:  "No reply to any previous message — try a different channel",
				Signal: "high", Color: "red",
			})
		case "negotiating":
			score += 0.15
			factors = append(factors, scoreFactor{
				Label:  "In active negotiation — lock in a specific date before momentum fades",
				Signal: "high", Color: "amber",
			})
		case "promised":
			factors = append(factors, scoreFactor{
				Label:  "Committed to pay — confirm receipt and close this out",
				Signal: "med", Color: "green",
			})
		}

		if score > 1.0 {
			score = 1.0
		} else if score < 0.0 {
			score = 0.0
		}

		outcomeText, outcomeTimeframe := buildOutcomeText(
			inv, customer, risk, daysOverdue, todayName, tomorrowName, now,
		)

		factorsJSON, err := json.Marshal(factors)
		if err != nil {
			factorsJSON = json.RawMessage(`[]`)
		}

		return db.InvoiceScore{
			ConfidenceScore:   score,
			ConfidenceFactors: factorsJSON,
			OutcomeText:       outcomeText,
			OutcomeTimeframe:  outcomeTimeframe,
		}, nil
	}
}

//nolint:gocognit,gocyclo // scoring outcome text is inherently complex; splitting would obscure the business rules
func buildOutcomeText(
	inv db.Invoice,
	customer db.Customer,
	risk db.CustomerRiskProfile,
	daysOverdue int,
	todayName, tomorrowName string,
	now time.Time,
) (text, timeframe string) {
	firstName := firstWord(customer.Name)

	// Best-day logic: "She usually pays after a firm message on Fridays — today is Friday"
	if risk.BestContactDay != nil && *risk.BestContactDay != "" {
		best := *risk.BestContactDay
		if strings.EqualFold(best, todayName) {
			if string(inv.CollectionState) == collectionStateNoResponse && inv.FollowUpCount >= 2 {
				return fmt.Sprintf(
					"%s usually responds on %ss — today is %s. Previous polite messages haven't worked, send a firmer one now.",
					firstName, best, best,
				), timeframeToday
			}
			return fmt.Sprintf(
				"Today is %s — %s's best day to respond. Send now for the highest chance of a reply.",
				best, firstName,
			), timeframeToday
		}
		if strings.EqualFold(best, tomorrowName) && daysOverdue <= 14 {
			return fmt.Sprintf(
				"%s responds best on %ss — that's tomorrow. Hold off today unless the amount is urgent.",
				firstName, best,
			), "Tomorrow"
		}
	}

	// No-response escalation
	if string(inv.CollectionState) == collectionStateNoResponse {
		if inv.FollowUpCount >= 3 {
			return fmt.Sprintf(
				"%d messages sent, no reply from %s. Polite isn't working — switch to a firm tone or try calling directly.",
				inv.FollowUpCount, firstName,
			), "Urgent"
		}
		if inv.FollowUpCount >= 2 {
			return fmt.Sprintf(
				"No reply after %d reminders. Send a firmer, more direct message now.",
				inv.FollowUpCount,
			), timeframeToday
		}
	}

	// Negotiating — press for a date
	if string(inv.CollectionState) == "negotiating" {
		return fmt.Sprintf(
			"%s is in negotiation — push for a specific date today. Ask: \"When exactly can you pay?\" Don't let this cool off.",
			firstName,
		), timeframeToday
	}

	// Promised — confirm
	if string(inv.CollectionState) == "promised" {
		if inv.PromisedAt != nil {
			daysSince := int(now.Sub(*inv.PromisedAt).Hours() / 24)
			if daysSince >= 3 {
				return fmt.Sprintf(
					"%s promised to pay %d days ago. Follow up now to confirm it's been done.",
					firstName, daysSince,
				), timeframeToday
			}
		}
		return fmt.Sprintf("%s has committed to pay. Keep an eye out and confirm when it arrives.", firstName), timeframeThisWeek
	}

	// Partial payment — nudge for remainder
	if inv.PaidAmount > 0 && inv.PaidAmount < inv.Amount {
		paidPct := int(float64(inv.PaidAmount) / float64(inv.Amount) * 100)
		return fmt.Sprintf(
			"%s has already paid %d%% of this invoice. A friendly reminder for the balance is all that's needed.",
			firstName, paidPct,
		), timeframeThisWeek
	}

	// History: usually pays fast but late this time
	if risk.AvgDaysToPay != nil && *risk.AvgDaysToPay <= 7 && daysOverdue > 0 {
		return fmt.Sprintf(
			"%s normally pays within %d days — this invoice is already %d days late. Something may have slipped. A polite check-in is worth it.",
			firstName, *risk.AvgDaysToPay, daysOverdue,
		), timeframeToday
	}

	// Always on time, now late
	if risk.InvoicesOnTime > 0 && risk.InvoicesLate == 0 && daysOverdue > 0 {
		return fmt.Sprintf(
			"%s has always paid on time before — this delay is unusual. A quick, friendly message is the right move.",
			firstName,
		), timeframeToday
	}

	// Long overdue
	if daysOverdue > 60 {
		return fmt.Sprintf(
			"%d days overdue. Polite reminders are no longer enough — %s needs a firm, direct message with a deadline.",
			daysOverdue, firstName,
		), "Urgent"
	}
	if daysOverdue > 30 {
		return fmt.Sprintf(
			"%d days past the due date. Send a firm but respectful reminder — the longer you leave it, the harder it gets.",
			daysOverdue,
		), timeframeThisWeek
	}
	if daysOverdue > 7 {
		return fmt.Sprintf("%d days since the due date. A clear, polite reminder is timely.", daysOverdue), timeframeThisWeek
	}

	// First contact
	if inv.FollowUpCount == 0 {
		if daysOverdue <= 0 {
			return fmt.Sprintf("Invoice is due soon — send the first reminder so %s has time to arrange payment.", firstName), timeframeThisWeek
		}
		return fmt.Sprintf("No contact made yet. Send the first reminder to %s now.", firstName), timeframeToday
	}

	// Reminded, awaiting response
	if inv.LastContactedAt != nil {
		silentDays := int(now.Sub(*inv.LastContactedAt).Hours() / 24)
		if silentDays >= 5 {
			return fmt.Sprintf("Last contacted %d days ago with no reply. Follow up now to stay on their radar.", silentDays), timeframeToday
		}
		return fmt.Sprintf("Reminder sent %d days ago. Give it until tomorrow, then follow up if no response.", silentDays), "Tomorrow"
	}

	return "Send a polite reminder. The sooner you reach out, the more likely a prompt reply.", timeframeThisWeek
}

func firstWord(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return name
	}
	return parts[0]
}
