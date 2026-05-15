package db

import (
	"context"
	"encoding/json"
	"fmt"
)

// InvoiceScore is the AI-computed confidence result for a single invoice.
// The Scorer callback is responsible for calling the AI gateway and
// mapping the response into this struct.
type InvoiceScore struct {
	ConfidenceScore   float64         // 0.0–1.0
	ConfidenceFactors json.RawMessage // [{label, signal, color: "green"|"amber"|"red"}]
	OutcomeText       string          // e.g. "Send now → paid today by 6 pm"
	OutcomeTimeframe  string          // "Today" | "This week" | "Next week"
}

// Scorer computes a confidence score for a single open invoice.
// The background job injects the real scorer; tests can inject a stub.
// The full ListUnscoredRow is passed so the scorer can load customer and
// risk-profile data without an extra query to resolve the user_id.
type Scorer func(ctx context.Context, row ListUnscoredRow) (InvoiceScore, error)

// RefreshRecoveryQueueTx scores a batch of open invoices whose confidence is
// stale or missing, writing the AI results back to the invoices table.
// Each invoice update is a separate single-row write (no outer transaction)
// so a scorer failure for one invoice never rolls back others.

type RefreshRecoveryQueueTxParams struct {
	Limit  int32  // invoices to process per run (e.g. 20)
	Scorer Scorer // injected by the caller
}

type RefreshRecoveryQueueTxResult struct {
	Processed int
	Errors    []error
}

func (s *SQLStore) RefreshRecoveryQueueTx(ctx context.Context, arg RefreshRecoveryQueueTxParams) (RefreshRecoveryQueueTxResult, error) {
	var result RefreshRecoveryQueueTxResult

	rows, err := s.ListUnscored(ctx, arg.Limit)
	if err != nil {
		return result, fmt.Errorf("list unscored: %w", err)
	}

	for _, row := range rows {
		score, scoreErr := arg.Scorer(ctx, row)
		if scoreErr != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("score invoice %s: %w", row.ID, scoreErr))
			continue
		}

		n, updateErr := s.UpdateInvoiceConfidence(ctx, UpdateInvoiceConfidenceParams{
			ID:                row.ID,
			UserID:            row.UserID,
			ConfidenceScore:   &score.ConfidenceScore,
			ConfidenceFactors: &score.ConfidenceFactors,
			OutcomeText:       &score.OutcomeText,
			OutcomeTimeframe:  &score.OutcomeTimeframe,
		})
		if updateErr != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("update invoice %s: %w", row.ID, updateErr))
			continue
		}
		if n == 0 {
			result.Errors = append(result.Errors,
				fmt.Errorf("update invoice %s: not found or already deleted", row.ID))
			continue
		}
		result.Processed++
	}

	return result, nil
}
