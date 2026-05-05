package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProcessWebhookTx_Success(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	eventID := uuid.New().String()

	res, err := testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  eventID,
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "card",
		},
	})

	require.NoError(t, err)

	require.Equal(t, inv.ID, res.Invoice.ID)
	require.Equal(t, int64(100), res.Payment.Amount)
	require.Equal(t, "paid", res.Invoice.Status)
}

// -----------------------------
// IDEMPOTENCY (duplicate webhook)
// -----------------------------

func TestProcessWebhookTx_DuplicateEvent(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	eventID := uuid.New().String()

	arg := ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  eventID,
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "card",
		},
	}

	_, err := testStore.ProcessWebhookTx(context.Background(), arg)
	require.NoError(t, err)

	// second call → should fail (unique constraint)
	_, err = testStore.ProcessWebhookTx(context.Background(), arg)
	require.Error(t, err)
}

// -----------------------------
// PARTIAL PAYMENT → PARTIAL STATUS
// -----------------------------

func TestProcessWebhookTx_PartialPayment(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  uuid.New().String(),
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "card",
		},
	})

	require.NoError(t, err)

	updated, err := testStore.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     inv.ID,
		UserID: user.ID,
	})
	require.NoError(t, err)

	require.Equal(t, "partial", updated.Status)
}

// -----------------------------
// MULTIPLE PAYMENTS ACCUMULATION
// -----------------------------

func TestProcessWebhookTx_MultiplePayments(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	// first
	_, _ = testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  uuid.New().String(),
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "card",
		},
	})

	// second
	_, _ = testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  uuid.New().String(),
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    60,
			Currency:  "THB",
			Method:    "card",
		},
	})

	final := getInvoice(t, inv.ID, user.ID)

	require.Equal(t, "paid", final.Status)
}

// -----------------------------
// INVALID PAYMENT (FK)
// -----------------------------

func TestProcessWebhookTx_InvalidPayment(t *testing.T) {
	user := committedUser(t)

	_, err := testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  uuid.New().String(),
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: uuid.New(), // invalid
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "card",
		},
	})

	require.Error(t, err)
}

// -----------------------------
// ATOMICITY (payment fails → no event)
// -----------------------------

func TestProcessWebhookTx_RollbackOnPaymentFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	eventID := uuid.New().String()

	_, err := testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  eventID,
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    -100, // invalid amount
			Currency:  "THB",
			Method:    "card",
		},
	})

	require.Error(t, err)

	// ensure webhook NOT inserted (rollback worked)
	_, err = testStore.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: "stripe",
		EventID:  eventID,
	})

	require.Error(t, err)
}

// -----------------------------
// WRONG USER (SECURITY)
// -----------------------------

func TestProcessWebhookTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.ProcessWebhookTx(context.Background(), ProcessWebhookTxParams{
		WebhookEvent: CreateWebhookEventParams{
			Provider: "stripe",
			EventID:  uuid.New().String(),
			Payload:  []byte(`{}`),
		},
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    other.ID, // wrong user
			Amount:    100,
			Currency:  "THB",
			Method:    "card",
		},
	})

	require.Error(t, err)
}
