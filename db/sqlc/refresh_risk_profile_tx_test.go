package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRefreshRiskProfileTx_FirstRun(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:        user.ID,
			CustomerID:    customer.ID,
			TotalInvoices: 5,
			RiskScore:     80,
			LateFeePct:    "0",
			LateFeeType:   LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
			EventValue: toJSON(t, map[string]any{
				"score": 80,
			}),
		},
	}

	res, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)

	require.Equal(t, int32(80), res.Profile.RiskScore)
	require.Equal(t, CustomerEventTypeRiskScoreChanged, res.Event.EventType)
}
func TestRefreshRiskProfileTx_Update(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	// initial
	_, _ = testStore.UpsertCustomerRiskProfile(context.Background(), UpsertCustomerRiskProfileParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		RiskScore:   70,
		LateFeePct:  "0",
		LateFeeType: LateFeeTypeEnumFlat,
	})

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			RiskScore:   30,
			LateFeePct:  "0",
			LateFeeType: LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	res, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)

	require.Equal(t, int32(30), res.Profile.RiskScore)
}
func TestRefreshRiskProfileTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:      other.ID, // wrong
			CustomerID:  customer.ID,
			RiskScore:   50,
			LateFeePct:  "0",
			LateFeeType: LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     other.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	_, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.Error(t, err)
}
func TestRefreshRiskProfileTx_RollbackOnEventFail(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			RiskScore:  60,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: uuid.New(), // invalid FK
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	_, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.Error(t, err)

	// verify rollback (profile should NOT exist)
	_, err = testStore.GetCustomerRiskProfile(context.Background(), GetCustomerRiskProfileParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.Error(t, err)
}

func TestRefreshRiskProfileTx_ZeroScore(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			RiskScore:   0,
			LateFeePct:  "0",
			LateFeeType: LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	res, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)

	require.Equal(t, int32(0), res.Profile.RiskScore)
}
func TestRefreshRiskProfileTx_MaxScore(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			RiskScore:   100,
			LateFeePct:  "0",
			LateFeeType: LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	res, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)

	require.Equal(t, int32(100), res.Profile.RiskScore)
}
func TestRefreshRiskProfileTx_Idempotent(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	params := RefreshRiskProfileTxParams{
		Profile: UpsertCustomerRiskProfileParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			RiskScore:   50,
			LateFeePct:  "0",
			LateFeeType: LateFeeTypeEnumFlat,
		},
		EventParams: CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			EventType:  CustomerEventTypeRiskScoreChanged,
		},
	}

	_, err := testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)

	_, err = testStore.RefreshRiskProfileTx(context.Background(), params)
	require.NoError(t, err)
}
