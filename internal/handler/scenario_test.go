package handler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"cashpilotx/internal/dto"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// dbStatusCancelled is the DB enum value — British spelling intentional.
const dbStatusCancelled = "cancelled" //nolint:misspell // DB enum value uses British spelling

// TestAPIScenario_CreateCustomerAndInvoice covers the core path a frontend user
// takes: register (seed) → create customer → create invoice → GET invoice.
func TestAPIScenario_CreateCustomerAndInvoice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP integration test in short mode")
	}

	env := newTestEnv(t)
	_, tok := env.seedUser(t)

	// Step 1: create a customer.
	var customer dto.CustomerResponse
	env.mustDo(t, http.MethodPost, "/api/v1/customers", map[string]any{
		"name": fmt.Sprintf("Scenario Customer %s", uuid.New().String()[:8]),
		"type": "customer",
		"tags": []string{},
	}, tok, http.StatusCreated, &customer)
	require.NotZero(t, customer.ID)
	require.Equal(t, "active", customer.Status)

	// Step 2: create an invoice for that customer.
	var created dto.CreateInvoiceResponse
	env.mustDo(t, http.MethodPost, "/api/v1/invoices", map[string]any{
		"customer_id":  customer.ID,
		"amount":       150000,
		"currency":     "THB",
		"due_date":     time.Now().Add(7 * 24 * time.Hour),
		"tone":         "polite",
		"channel":      "whatsapp",
		"max_attempts": 3,
	}, tok, http.StatusCreated, &created)
	require.NotZero(t, created.Invoice.ID)
	require.Equal(t, "unpaid", created.Invoice.Status)
	require.Equal(t, "pending", created.Invoice.CollectionState)
	require.NotNil(t, created.Reminder)

	// Step 3: GET invoice — verify it's retrievable.
	var fetched dto.InvoiceResponse
	env.mustDo(t, http.MethodGet, "/api/v1/invoices/"+created.Invoice.ID.String(), nil, tok, http.StatusOK, &fetched)
	require.Equal(t, created.Invoice.ID, fetched.ID)
	require.Equal(t, int64(150000), fetched.Amount)
}

// TestAPIScenario_InvoicePaymentLifecycle covers the full pay-off flow via HTTP:
// create customer → create invoice → record partial payment → record final payment.
func TestAPIScenario_InvoicePaymentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP integration test in short mode")
	}

	env := newTestEnv(t)
	_, tok := env.seedUser(t)

	// Create customer.
	var customer dto.CustomerResponse
	env.mustDo(t, http.MethodPost, "/api/v1/customers", map[string]any{
		"name": fmt.Sprintf("Pay Scenario %s", uuid.New().String()[:8]),
		"type": "customer",
		"tags": []string{},
	}, tok, http.StatusCreated, &customer)

	// Create invoice (amount = 1000 THB).
	var created dto.CreateInvoiceResponse
	env.mustDo(t, http.MethodPost, "/api/v1/invoices", map[string]any{
		"customer_id":  customer.ID,
		"amount":       1000,
		"currency":     "THB",
		"due_date":     time.Now().Add(7 * 24 * time.Hour),
		"tone":         "polite",
		"channel":      "whatsapp",
		"max_attempts": 3,
	}, tok, http.StatusCreated, &created)
	invoiceID := created.Invoice.ID

	// Record partial payment (400 THB).
	var partial dto.RecordPaymentResponse
	env.mustDoIdempotent(t, http.MethodPost, "/api/v1/payments", map[string]any{
		"invoice_id":  invoiceID,
		"customer_id": customer.ID,
		"amount":      400,
		"method":      "cash",
	}, tok, http.StatusCreated, &partial)
	require.Equal(t, "partial", partial.Invoice.Status)
	require.Equal(t, int64(400), partial.Invoice.PaidAmount)

	// Record final payment (600 THB) → invoice paid.
	var final dto.RecordPaymentResponse
	env.mustDoIdempotent(t, http.MethodPost, "/api/v1/payments", map[string]any{
		"invoice_id":  invoiceID,
		"customer_id": customer.ID,
		"amount":      600,
		"method":      "transfer",
	}, tok, http.StatusCreated, &final)
	require.Equal(t, "paid", final.Invoice.Status)
	require.Equal(t, "paid", final.Invoice.CollectionState)
	require.Equal(t, int64(1000), final.Invoice.PaidAmount)

	// GET invoice should reflect paid state.
	var fetched dto.InvoiceResponse
	env.mustDo(t, http.MethodGet, "/api/v1/invoices/"+invoiceID.String(), nil, tok, http.StatusOK, &fetched)
	require.Equal(t, "paid", fetched.Status)
}

// TestAPIScenario_CancelAndReactivate exercises the cancel → reactivate cycle via HTTP.
func TestAPIScenario_CancelAndReactivate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP integration test in short mode")
	}

	env := newTestEnv(t)
	_, tok := env.seedUser(t)

	// Create customer.
	var customer dto.CustomerResponse
	env.mustDo(t, http.MethodPost, "/api/v1/customers", map[string]any{
		"name": fmt.Sprintf("Cancel Scenario %s", uuid.New().String()[:8]),
		"type": "customer",
		"tags": []string{},
	}, tok, http.StatusCreated, &customer)

	// Create invoice.
	var created dto.CreateInvoiceResponse
	env.mustDo(t, http.MethodPost, "/api/v1/invoices", map[string]any{
		"customer_id":  customer.ID,
		"amount":       500,
		"currency":     "THB",
		"due_date":     time.Now().Add(24 * time.Hour),
		"tone":         "polite",
		"channel":      "whatsapp",
		"max_attempts": 3,
	}, tok, http.StatusCreated, &created)
	invoiceID := created.Invoice.ID

	// Cancel the invoice.
	var canceled dto.InvoiceResponse
	env.mustDo(t, http.MethodPost, "/api/v1/invoices/"+invoiceID.String()+"/cancel", nil, tok, http.StatusOK, &canceled)
	require.Equal(t, dbStatusCancelled, canceled.Status)

	// Reactivate it.
	var reactivated dto.InvoiceResponse
	env.mustDo(t, http.MethodPost, "/api/v1/invoices/"+invoiceID.String()+"/reactivate", nil, tok, http.StatusOK, &reactivated)
	require.Equal(t, "unpaid", reactivated.Status)
	require.Equal(t, "pending", reactivated.CollectionState)
}

// TestAPIScenario_UnauthorizedAccessBlocked verifies that all protected endpoints
// return 401 when no token is provided.
func TestAPIScenario_UnauthorizedAccessBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP integration test in short mode")
	}

	env := newTestEnv(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/customers"},
		{http.MethodGet, "/api/v1/invoices"},
		{http.MethodGet, "/api/v1/dashboard"},
		{http.MethodGet, "/api/v1/sync"},
	}

	for _, ep := range endpoints {
		got := env.mustGetStatus(t, ep.method, ep.path, "")
		require.Equal(t, http.StatusUnauthorized, got,
			"expected 401 for %s %s", ep.method, ep.path)
	}
}
