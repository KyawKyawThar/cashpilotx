package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRefreshRecoveryQueueTx_AllSuccess(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	// create 3 invoices (unscored)
	invoices := make([]Invoice, 3)
	for i := range 3 {
		invoices[i] = committedInvoice(t, user.ID, customer.ID)
	}

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		return InvoiceScore{
			ConfidenceScore:   0.9,
			ConfidenceFactors: json.RawMessage(`[{"label":"good","signal":"high"}]`),
			OutcomeText:       "Pay today",
			OutcomeTimeframe:  "Today",
		}, nil
	}

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 3, res.Processed)
	require.Empty(t, res.Errors)
}

func TestRefreshRecoveryQueueTx_PartialFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	invoices := make([]Invoice, 3)
	for i := range 3 {
		invoices[i] = committedInvoice(t, user.ID, customer.ID)
	}

	callCount := 0

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		callCount++
		if callCount == 2 {
			return InvoiceScore{}, errors.New("AI failure")
		}
		return InvoiceScore{
			ConfidenceScore:   0.8,
			ConfidenceFactors: json.RawMessage(`[]`),
			OutcomeText:       "ok",
			OutcomeTimeframe:  "Today",
		}, nil
	}

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 2, res.Processed)
	require.Len(t, res.Errors, 1)
}
func TestRefreshRecoveryQueueTx_UpdateFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	inv := committedInvoice(t, user.ID, customer.ID)

	// Delete the invoice inside the scorer so ListUnscored still finds it,
	// but UpdateInvoiceConfidence matches 0 rows and records an error.
	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		_, _ = testDB.Exec(ctx, "DELETE FROM invoices WHERE id = $1", id)
		return InvoiceScore{
			ConfidenceScore:   0.7,
			ConfidenceFactors: json.RawMessage(`[]`),
			OutcomeText:       "ok",
			OutcomeTimeframe:  "Today",
		}, nil
	}

	_ = inv

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 0, res.Processed)
	require.NotEmpty(t, res.Errors)
}
func TestRefreshRecoveryQueueTx_Limit(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	for range 5 {
		committedInvoice(t, user.ID, customer.ID)
	}

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		return InvoiceScore{
			ConfidenceScore: 0.5,
		}, nil
	}

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  2,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 2, res.Processed)
}
func TestRefreshRecoveryQueueTx_Empty(t *testing.T) {
	committedUser(t)

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		return InvoiceScore{}, nil
	}

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 0, res.Processed)
	require.Empty(t, res.Errors)
}
func TestRefreshRecoveryQueueTx_Idempotent(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	committedInvoice(t, user.ID, customer.ID)

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		return InvoiceScore{
			ConfidenceScore: 0.6,
		}, nil
	}

	// first run
	res1, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res1.Processed)

	// second run → should not process again (already scored)
	res2, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})
	require.NoError(t, err)

	require.Equal(t, 0, res2.Processed)
}
func TestRefreshRecoveryQueueTx_MultipleErrors(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	for range 3 {
		committedInvoice(t, user.ID, customer.ID)
	}

	scorer := func(ctx context.Context, id uuid.UUID) (InvoiceScore, error) {
		return InvoiceScore{}, errors.New("fail")
	}

	res, err := testStore.RefreshRecoveryQueueTx(context.Background(), RefreshRecoveryQueueTxParams{
		Limit:  10,
		Scorer: scorer,
	})

	require.NoError(t, err)
	require.Equal(t, 0, res.Processed)
	require.Len(t, res.Errors, 3)
}
