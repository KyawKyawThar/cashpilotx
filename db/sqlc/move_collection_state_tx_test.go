package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func committedInvoice(t *testing.T, userID, customerID uuid.UUID) Invoice {
	t.Helper()

	inv, err := testStore.CreateInvoice(context.Background(), CreateInvoiceParams{
		UserID:     userID,
		CustomerID: customerID,
		Amount:     100,
		Currency:   "USD",
		DueDate:    time.Now().Add(24 * time.Hour),
		Column10:   json.RawMessage("[]"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(),
			"DELETE FROM invoices WHERE id = $1", inv.ID)
	})

	return inv
}

func getInvoice(t *testing.T, id, userID uuid.UUID) Invoice {
	t.Helper()

	inv, err := testStore.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     id,
		UserID: userID,
	})
	require.NoError(t, err)

	return inv
}
func TestMoveCollectionStateTx_Success_NoNote(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	res, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "reminded",
	})

	require.NoError(t, err)
	require.Equal(t, "reminded", res.Invoice.CollectionState)
	require.NotZero(t, res.Event.ID)
}
func TestMoveCollectionStateTx_Success_WithNote(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	note := "customer will pay tomorrow"

	res, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "promised",
		Note:            &note,
	})

	require.NoError(t, err)
	require.Contains(t, string(*res.Event.EventValue), "note")
}
func TestMoveCollectionStateTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          other.ID,
		CustomerID:      customer.ID,
		CollectionState: "reminded",
	})

	require.Error(t, err)
}
func TestMoveCollectionStateTx_InvalidState(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "INVALID_STATE",
	})

	require.Error(t, err)
}
func TestMoveCollectionStateTx_RollbackOnEventFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      uuid.New(), // invalid FK → event fails
		CollectionState: "reminded",
	})

	require.Error(t, err)

	// ensure rollback worked
	updated := getInvoice(t, inv.ID, user.ID)
	require.NotEqual(t, "reminded", updated.CollectionState)
}
func TestMoveCollectionStateTx_IdempotentSameState(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "reminded",
	})
	require.NoError(t, err)

	// same state again
	_, err = testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
		InvoiceID:       inv.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "reminded",
	})
	require.NoError(t, err)
}

func TestMoveCollectionStateTx_RapidTransitions(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	states := []string{"reminded", "negotiating", "promised", "defaulted"}

	for _, s := range states {
		_, err := testStore.MoveCollectionStateTx(context.Background(), MoveCollectionStateTxParams{
			InvoiceID:       inv.ID,
			UserID:          user.ID,
			CustomerID:      customer.ID,
			CollectionState: s,
		})
		require.NoError(t, err)
	}

	final := getInvoice(t, inv.ID, user.ID)
	require.Equal(t, "defaulted", final.CollectionState)
}
