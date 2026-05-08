package db

import (
	"cashpilotx/utils"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// committedUser creates a real committed user via testStore and registers cleanup.
// Required for Tx tests whose execTx opens a separate pool connection and cannot
// see rows in an uncommitted test transaction.
func committedUser(t *testing.T) User {
	t.Helper()
	bn := utils.RandomString(8)
	local := "en"
	u, err := testStore.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(9),
		PinHash:      "test_hash",
		Name:         utils.RandomString(6),
		BusinessName: &bn,
		Locale:       &local,
		Plan:         "pro",
		HomeCurrency: "THB",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
	})
	return u
}

// committedCustomer creates a real committed customer via testStore and registers cleanup.
func committedCustomer(t *testing.T, userID uuid.UUID) Customer {
	t.Helper()
	name := utils.RandomString(6)
	country := "TH"
	c, err := testStore.CreateCustomer(context.Background(), CreateCustomerParams{
		UserID:         userID,
		Name:           name,
		NormalizedName: strings.ToLower(name),
		Country:        &country,
		Type:           "customer",
		Tags:           []string{},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM customers WHERE id = $1", c.ID)
	})
	return c
}

func TestCreateInvoiceTx_Success(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	result, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	inv := result.Invoice
	require.NotZero(t, inv.ID)
	require.Equal(t, user.ID, inv.UserID)
	require.Equal(t, customer.ID, inv.CustomerID)
	require.Equal(t, int64(500), inv.Amount)
	require.Equal(t, "unpaid", inv.Status)
	require.NotZero(t, result.Reminder.ID)
}

func TestCreateInvoiceTx_WithPartialPayment(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	result, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	updated, err := testStore.AddPayment(context.Background(), AddPaymentParams{
		ID:         result.Invoice.ID,
		UserID:     user.ID,
		PaidAmount: 200,
	})
	require.NoError(t, err)
	require.Equal(t, int64(200), updated.PaidAmount)
}
func TestCreateInvoiceTx_RollbackOnReminderFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	// invalid reminder (missing required fields like channel)
	_, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID: user.ID,
			// Channel missing → should fail
		},
	})

	require.Error(t, err)

	// ensure NOTHING was created (atomicity)
	invoices, err := testStore.ListInvoices(context.Background(), ListInvoicesParams{
		UserID: user.ID,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Empty(t, invoices)
}
func TestCreateInvoiceTx_InvalidCustomer(t *testing.T) {
	user := committedUser(t)

	_, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: uuid.New(), // invalid
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID: user.ID,
		},
	})

	require.Error(t, err)
}
func TestCreateInvoiceTx_WithNewCustomer(t *testing.T) {
	user := committedUser(t)

	result, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		NewCustomer: &CreateCustomerParams{
			UserID: user.ID,
			Name:   "new_tx_customer",
			Type:   "customer",
		},
		Invoice: CreateInvoiceParams{
			UserID:   user.ID,
			Amount:   800,
			Currency: "THB",
			DueDate:  time.Now().Add(24 * time.Hour),
			Column10: json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(time.Hour),
		},
	})

	require.NoError(t, err)
	require.NotZero(t, result.Customer.ID)
	require.Equal(t, result.Customer.ID, result.Invoice.CustomerID)
}
func TestCancelInvoiceTx_Success(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	res, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     1000,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(time.Hour),
		},
	})
	require.NoError(t, err)

	result, err := testStore.CancelInvoiceTx(context.Background(), CancelInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "cancelled", result.Invoice.Status) //nolint:misspell // DB constraint uses British spelling
}
func TestCancelInvoiceTx_AlreadyCanceled(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	res, _ := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID: user.ID,
		},
	})

	_, _ = testStore.CancelInvoiceTx(context.Background(), CancelInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	_, err := testStore.CancelInvoiceTx(context.Background(), CancelInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvoiceAlreadyCanceled)
}

func TestCancelInvoiceTx_PartialPayment(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	res, _ := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     1000,
			Currency:   "THB",
			DueDate:    time.Now().Add(time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(time.Hour),
		},
	})

	// pay partially
	_, _ = testStore.AddPayment(context.Background(), AddPaymentParams{
		ID:         res.Invoice.ID,
		UserID:     user.ID,
		PaidAmount: 400,
	})

	result, err := testStore.CancelInvoiceTx(context.Background(), CancelInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.NoError(t, err)

	// unpaid = 600 should be reversed (not full 1000)
	require.Equal(t, int64(1000), result.Invoice.Amount)
	require.Equal(t, int64(400), result.Invoice.PaidAmount)
}
func TestWriteOffInvoiceTx_Success(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	res, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     700,
			Currency:   "THB",
			DueDate:    time.Now().Add(time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	result, err := testStore.WriteOffInvoiceTx(context.Background(), WriteOffInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "defaulted", result.Invoice.CollectionState)
	require.Equal(t, "cancelled", result.Invoice.Status) //nolint:misspell // DB constraint uses British spelling
}
func TestWriteOffInvoiceTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)

	res, _ := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now(),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID: user.ID,
		},
	})

	_, err := testStore.WriteOffInvoiceTx(context.Background(), WriteOffInvoiceTxParams{
		InvoiceID:  res.Invoice.ID,
		UserID:     other.ID, // wrong user
		CustomerID: customer.ID,
	})

	require.Error(t, err)
}
