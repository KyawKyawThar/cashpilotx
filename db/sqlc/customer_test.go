package db

import (
	"cashpilotx/utils"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestCustomer(t *testing.T, store *SQLStore, userID uuid.UUID) Customer {
	t.Helper()

	name := utils.RandomString(6)
	norm := strings.ToLower(name)
	country := "TH"
	c, err := store.CreateCustomer(context.Background(), CreateCustomerParams{
		UserID:         userID,
		Name:           name,
		NormalizedName: norm,
		Country:        &country,
		Type:           "customer",
		Tags:           []string{},
	})

	require.NoError(t, err)
	return c
}
func TestCreateCustomer_Normalization(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	name := "John Doe"
	norm := "john doe"

	country := "TH"

	c, err := store.CreateCustomer(context.Background(), CreateCustomerParams{
		UserID:         user.ID,
		Name:           name,
		NormalizedName: norm,
		Country:        &country,
		Type:           "customer",
	})

	require.NoError(t, err)
	require.Equal(t, norm, c.NormalizedName)
}
func TestAddCustomerDebt_Accumulates(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	c := createTestCustomer(t, store, user.ID)

	updated, err := store.AddCustomerDebt(context.Background(), AddCustomerDebtParams{
		ID:        c.ID,
		UserID:    user.ID,
		TotalOwed: 100,
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), updated.TotalOwed)

	updated, _ = store.AddCustomerDebt(context.Background(), AddCustomerDebtParams{
		ID:        c.ID,
		UserID:    user.ID,
		TotalOwed: 50,
	})

	require.Equal(t, int64(150), updated.TotalOwed)
}

func TestAddCustomerPayment_NoNegative(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	c := createTestCustomer(t, store, user.ID)

	// no debt yet
	updated, err := store.AddCustomerPayment(context.Background(), AddCustomerPaymentParams{
		ID:        c.ID,
		UserID:    user.ID,
		TotalPaid: 100,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), updated.TotalOwed) // must not go negative
}
func TestCustomer_Isolation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	u1 := createTestUser(t, store)
	u2 := createTestUser(t, store)

	c := createTestCustomer(t, store, u1.ID)

	_, err := store.GetCustomerByID(context.Background(), GetCustomerByIDParams{
		ID:     c.ID,
		UserID: u2.ID,
	})

	require.Error(t, err)
}
func TestSearchCustomers(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	name := "Alice"
	norm := "alice"

	country := "TH"

	_, _ = store.CreateCustomer(context.Background(), CreateCustomerParams{
		UserID:         user.ID,
		Name:           name,
		NormalizedName: norm,
		Country:        &country,
		Type:           "customer",
	})

	query := "ali"

	results, err := store.SearchCustomers(context.Background(), SearchCustomersParams{
		UserID:  user.ID,
		Column2: &query,
	})

	require.NoError(t, err)
	require.NotEmpty(t, results)
}
func TestListCustomers_Pagination(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	for range 3 {
		createTestCustomer(t, store, user.ID)
	}

	list, err := store.ListCustomers(context.Background(), ListCustomersParams{
		UserID: user.ID,
		Limit:  2,
		Offset: 0,
	})

	require.NoError(t, err)
	require.Len(t, list, 2)
}
func TestListCustomers_OnlyActive(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	c := createTestCustomer(t, store, user.ID)

	// deactivate
	_ = store.SetCustomerStatus(context.Background(), SetCustomerStatusParams{
		ID:     c.ID,
		UserID: user.ID,
		Status: "inactive",
	})

	list, err := store.ListCustomers(context.Background(), ListCustomersParams{
		UserID: user.ID,
		Limit:  10,
	})

	require.NoError(t, err)

	for _, x := range list {
		require.Equal(t, "active", x.Status)
	}
}
func TestTopDebtors_Order(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	c1 := createTestCustomer(t, store, user.ID)
	c2 := createTestCustomer(t, store, user.ID)

	_, _ = store.AddCustomerDebt(context.Background(), AddCustomerDebtParams{
		ID:        c1.ID,
		UserID:    user.ID,
		TotalOwed: 50,
	})

	_, _ = store.AddCustomerDebt(context.Background(), AddCustomerDebtParams{
		ID:        c2.ID,
		UserID:    user.ID,
		TotalOwed: 200,
	})

	list, err := store.TopDebtors(context.Background(), TopDebtorsParams{
		UserID: user.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Equal(t, c2.ID, list[0].ID)
}
func TestUpdateCustomer_Partial(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	c := createTestCustomer(t, store, user.ID)

	newName := "Updated"

	updated, err := store.UpdateCustomer(context.Background(), UpdateCustomerParams{
		ID:             c.ID,
		UserID:         user.ID,
		Name:           &newName,
		NormalizedName: &newName,
	})

	require.NoError(t, err)
	require.Equal(t, newName, updated.Name)
}
