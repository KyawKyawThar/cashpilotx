package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestRiskProfile(t *testing.T, store *SQLStore, userID, customerID uuid.UUID) CustomerRiskProfile {
	t.Helper()

	avgPay := int32(5)

	arg := UpsertCustomerRiskProfileParams{
		UserID:         userID,
		CustomerID:     customerID,
		TotalInvoices:  10,
		RiskScore:      75,
		AvgDaysToPay:   &avgPay,
		InvoicesOnTime: 7,
		InvoicesLate:   2,
		InvoicesUnpaid: 1,
		AmountOnTime:   7000,
		AmountLate:     2000,
		AmountUnpaid:   1000,
		LongestOverdue: 10,
		LateFeeEnabled: true,
		LateFeePct:     "2.5",
		LateFeeType:    LateFeeTypeEnumPercentage,
		TotalLateFees:  10000, // 100.00 in minor units
	}

	profile, err := store.UpsertCustomerRiskProfile(context.Background(), arg)
	require.NoError(t, err)
	return profile
}

func TestUpsertCustomerRiskProfile_Create(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	profile := createTestRiskProfile(t, store, user.ID, customer.ID)

	require.NotZero(t, profile.ID)
	require.Equal(t, user.ID, profile.UserID)
	require.Equal(t, customer.ID, profile.CustomerID)
	require.Equal(t, int32(75), profile.RiskScore)
	require.Equal(t, "good", profile.RiskLevel)
	require.NotZero(t, profile.CreatedAt)
}

func TestUpsertCustomerRiskProfile_Update(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	first := createTestRiskProfile(t, store, user.ID, customer.ID)

	avgPay := int32(5)
	arg := UpsertCustomerRiskProfileParams{
		UserID:         user.ID,
		CustomerID:     customer.ID,
		TotalInvoices:  20,
		RiskScore:      30, // downgrade
		AvgDaysToPay:   &avgPay,
		InvoicesOnTime: 10,
		InvoicesLate:   5,
		InvoicesUnpaid: 5,
		AmountOnTime:   10000,
		AmountLate:     5000,
		AmountUnpaid:   3000,
		LongestOverdue: 30,
		LateFeeEnabled: false,
		LateFeePct:     "2.5",
		LateFeeType:    LateFeeTypeEnumPercentage,
		TotalLateFees:  10000,
	}

	updated, err := store.UpsertCustomerRiskProfile(context.Background(), arg)
	require.NoError(t, err)

	require.Equal(t, first.ID, updated.ID)
	require.Equal(t, "risky", updated.RiskLevel)
	require.Equal(t, int32(30), updated.RiskScore)
}

func TestGetCustomerRiskProfile_OK(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	expected := createTestRiskProfile(t, store, user.ID, customer.ID)

	result, err := store.GetCustomerRiskProfile(context.Background(), GetCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.Equal(t, expected.ID, result.ID)
}

func TestGetCustomerRiskProfile_NotFound(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	_, err := store.GetCustomerRiskProfile(context.Background(), GetCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: uuid.New(),
	})

	require.Error(t, err)
}

func TestDeleteCustomerRiskProfile(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	createTestRiskProfile(t, store, user.ID, customer.ID)

	err := store.DeleteCustomerRiskProfile(context.Background(), DeleteCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)

	_, err = store.GetCustomerRiskProfile(context.Background(), GetCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.Error(t, err)
}

func TestListCustomerRiskProfiles_Pagination(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	for i := 0; i < 10; i++ {
		customer := createTestCustomer(t, store, user.ID)
		createTestRiskProfile(t, store, user.ID, customer.ID)
	}

	first, err := store.ListCustomerRiskProfiles(context.Background(), ListCustomerRiskProfilesParams{
		UserID: user.ID,
		Limit:  5,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, first, 5)

	second, err := store.ListCustomerRiskProfiles(context.Background(), ListCustomerRiskProfilesParams{
		UserID: user.ID,
		Limit:  5,
		Offset: 5,
	})
	require.NoError(t, err)
	require.Len(t, second, 5)

	require.NotEqual(t, first[0].ID, second[0].ID)
}

func TestListCustomerRiskProfiles_UserIsolation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	createTestRiskProfile(t, store, user.ID, customer.ID)

	result, err := store.ListCustomerRiskProfiles(context.Background(), ListCustomerRiskProfilesParams{
		UserID: otherUser.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}

func TestListBestCustomers_Filter(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)

	// excellent (score >= 80)
	_, err := store.UpsertCustomerRiskProfile(context.Background(), UpsertCustomerRiskProfileParams{
		UserID:        user.ID,
		CustomerID:    customer.ID,
		TotalInvoices: 10,
		RiskScore:     85,
		LateFeePct:    "0",
		TotalLateFees: 0,
		LateFeeType:   "flat",
	})
	require.NoError(t, err)

	result, err := store.ListBestCustomers(context.Background(), ListBestCustomersParams{
		UserID: user.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "excellent", result[0].RiskLevel)
}
func TestListHighRiskCustomers_Filter(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)

	_, err := store.UpsertCustomerRiskProfile(context.Background(), UpsertCustomerRiskProfileParams{
		UserID:        user.ID,
		CustomerID:    customer.ID,
		TotalInvoices: 10,
		RiskScore:     10, // bad
		LateFeePct:    "0",
		TotalLateFees: 0,
		LateFeeType:   "flat",
	})
	require.NoError(t, err)

	result, err := store.ListHighRiskCustomers(context.Background(), ListHighRiskCustomersParams{
		UserID: user.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Contains(t, []string{"risky", "bad"}, result[0].RiskLevel)
}

func TestUpdateLateFeeSettings_Validation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	createTestRiskProfile(t, store, user.ID, customer.ID)

	// invalid: enabled but pct = 0
	err := store.UpdateLateFeeSettings(context.Background(), UpdateLateFeeSettingsParams{
		UserID:         user.ID,
		CustomerID:     customer.ID,
		LateFeeEnabled: true,
		LateFeePct:     "0",
		LateFeeType:    LateFeeTypeEnumPercentage,
	})

	require.NoError(t, err) // SQL silently ignores (important edge case)
}

func TestTouchRiskProfileCalculated(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	profile := createTestRiskProfile(t, store, user.ID, customer.ID)

	time.Sleep(time.Millisecond * 10)

	err := store.TouchRiskProfileCalculated(context.Background(), TouchRiskProfileCalculatedParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)

	updated, err := store.GetCustomerRiskProfile(context.Background(), GetCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(profile.UpdatedAt))
}

func TestUpdateContactWindow_Nullable(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	createTestRiskProfile(t, store, user.ID, customer.ID)

	err := store.UpdateContactWindow(context.Background(), UpdateContactWindowParams{
		UserID:            user.ID,
		CustomerID:        customer.ID,
		BestContactDay:    nil,
		BestContactTime:   nil,
		BestContactWindow: nil,
	})

	require.NoError(t, err)
}
