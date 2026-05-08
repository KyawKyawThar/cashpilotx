package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func createTestCustomerInsight(t *testing.T, store *SQLStore, userID, customerID uuid.UUID, insightType InsightTypeEnum, text string) CustomerInsight {
	t.Helper()

	insight, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      userID,
		CustomerID:  customerID,
		InsightType: insightType,
		InsightText: text,
		Column10:    true,
	})
	require.NoError(t, err)
	require.NotZero(t, insight.ID)
	return insight
}

func TestCreateCustomerInsight_Defaults(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	before := time.Now()
	insight, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Customer often pays after several reminders",
		Column8:     time.Now(),
		Column10:    true,
	})
	after := time.Now()

	require.NoError(t, err)
	require.Equal(t, user.ID, insight.UserID)
	require.Equal(t, customer.ID, insight.CustomerID)
	require.Equal(t, InsightTypeEnumRisk, insight.InsightType)
	require.Equal(t, "Customer often pays after several reminders", insight.InsightText)
	require.Nil(t, insight.Score)
	require.Nil(t, insight.DeltaScore)
	require.JSONEq(t, `{}`, string(insight.Metadata))
	require.WithinDuration(t, before, insight.ValidFrom, after.Sub(before)+time.Second)
	require.Nil(t, insight.ValidTo)
	require.True(t, insight.IsCurrent)
	require.False(t, insight.CreatedAt.IsZero())
}

func TestCreateCustomerInsight_WithAllOptionalFields(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	score := "72.50"
	deltaScore := "-4.25"
	validFrom := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	validTo := time.Now().Add(24 * time.Hour).Truncate(time.Microsecond)
	metadata := map[string]any{
		"avg_days_late": 6,
		"evidence":      []string{"invoice_count", "reply_delay"},
	}

	created, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumPaymentBehavior,
		InsightText: "Payment behavior is improving",
		Score:       &score,
		DeltaScore:  &deltaScore,
		Column7:     *toJSON(t, metadata),
		Column8:     validFrom,
		ValidTo:     &validTo,
		Column10:    false,
	})
	require.NoError(t, err)

	fetched, err := store.GetCustomerInsightByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, score, *fetched.Score)
	require.Equal(t, deltaScore, *fetched.DeltaScore)
	require.JSONEq(t, `{"avg_days_late":6,"evidence":["invoice_count","reply_delay"]}`, string(fetched.Metadata))
	require.WithinDuration(t, validFrom, fetched.ValidFrom, time.Microsecond)
	require.WithinDuration(t, validTo, *fetched.ValidTo, time.Microsecond)
	require.False(t, fetched.IsCurrent)
}

func TestGetCurrentInsight_NotFoundForNonCurrentOrWrongTenant(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Historical risk note",
		Column10:    false,
	})
	require.NoError(t, err)

	_, err = store.GetCurrentInsight(context.Background(), GetCurrentInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	current := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumRisk, "Current risk note")
	require.True(t, current.IsCurrent)

	_, err = store.GetCurrentInsight(context.Background(), GetCurrentInsightParams{
		UserID:      otherUser.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestCreateCustomerInsight_RejectsSecondCurrentForSameType(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumEngagement, "Replies quickly")

	store2 := newTestStore(t)
	var err error
	_, err = store2.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumEngagement,
		InsightText: "Duplicate current engagement note",
		Column10:    false,
	})
	require.Error(t, err)
}

func TestCreateCustomerInsight_AllowsMultipleHistoricalRowsForSameType(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	for _, text := range []string{"Old note one", "Old note two"} {
		_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			InsightType: InsightTypeEnumPaymentBehavior,
			InsightText: text,
			Column10:    false,
		})
		require.NoError(t, err)
	}

	history, err := store.ListInsightsHistory(context.Background(), ListInsightsHistoryParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumPaymentBehavior,
	})
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.False(t, history[0].IsCurrent)
	require.False(t, history[1].IsCurrent)
}

func TestCloseCurrentInsight_ClosesOnlyMatchingCurrentInsight(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	otherCustomer := createTestCustomer(t, store, user.ID)
	validTo := time.Now().Add(-time.Hour).Truncate(time.Microsecond)

	target := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumRisk, "Target risk")
	otherType := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumEngagement, "Still current")
	otherCustomerInsight := createTestCustomerInsight(t, store, user.ID, otherCustomer.ID, InsightTypeEnumRisk, "Other customer")

	err := store.CloseCurrentInsight(context.Background(), CloseCurrentInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		ValidTo:     &validTo,
	})
	require.NoError(t, err)

	closed, err := store.GetCustomerInsightByID(context.Background(), target.ID)
	require.NoError(t, err)
	require.False(t, closed.IsCurrent)
	require.NotNil(t, closed.ValidTo)
	require.WithinDuration(t, validTo, *closed.ValidTo, time.Microsecond)

	stillCurrent, err := store.GetCustomerInsightByID(context.Background(), otherType.ID)
	require.NoError(t, err)
	require.True(t, stillCurrent.IsCurrent)
	require.Nil(t, stillCurrent.ValidTo)

	stillCurrent, err = store.GetCustomerInsightByID(context.Background(), otherCustomerInsight.ID)
	require.NoError(t, err)
	require.True(t, stillCurrent.IsCurrent)
	require.Nil(t, stillCurrent.ValidTo)
}

func TestCloseCurrentInsight_NoMatchIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	err := store.CloseCurrentInsight(context.Background(), CloseCurrentInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.NoError(t, err)
}

func TestReplaceInsightAtomic_ClosesOldAndInsertsNewCurrent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	oldValidFrom := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	old, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Old risk",
		Column8:     oldValidFrom,
		Column10:    true,
	})
	require.NoError(t, err)
	score := "88.00"

	replacement, err := store.ReplaceInsightAtomic(context.Background(), ReplaceInsightAtomicParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "New risk",
		Score:       &score,
		Column7:     json.RawMessage(`{"source":"daily_refresh"}`),
	})
	require.NoError(t, err)
	require.NotEqual(t, old.ID, replacement.ID)
	require.True(t, replacement.IsCurrent)
	require.Nil(t, replacement.ValidTo)
	require.Equal(t, score, *replacement.Score)
	require.JSONEq(t, `{"source":"daily_refresh"}`, string(replacement.Metadata))

	closedOld, err := store.GetCustomerInsightByID(context.Background(), old.ID)
	require.NoError(t, err)
	require.False(t, closedOld.IsCurrent)
	require.NotNil(t, closedOld.ValidTo)

	current, err := store.GetCurrentInsight(context.Background(), GetCurrentInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.NoError(t, err)
	require.Equal(t, replacement.ID, current.ID)

	history, err := store.ListInsightsHistory(context.Background(), ListInsightsHistoryParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, replacement.ID, history[0].ID)
	require.Equal(t, old.ID, history[1].ID)
}

func TestListCurrentInsightsByCustomer_FiltersAndOrdersByType(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	otherCustomer := createTestCustomer(t, store, user.ID)

	payment := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumPaymentBehavior, "Pays late")
	engagement := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumEngagement, "Answers messages")
	risk := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumRisk, "Medium risk")
	createTestCustomerInsight(t, store, user.ID, otherCustomer.ID, InsightTypeEnumRisk, "Other customer risk")

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Historical risk",
		Column10:    false,
	})
	require.NoError(t, err)

	list, err := store.ListCurrentInsightsByCustomer(context.Background(), ListCurrentInsightsByCustomerParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, []uuid.UUID{risk.ID, payment.ID, engagement.ID}, []uuid.UUID{list[0].ID, list[1].ID, list[2].ID})

	otherTenantList, err := store.ListCurrentInsightsByCustomer(context.Background(), ListCurrentInsightsByCustomerParams{
		UserID:     otherUser.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Empty(t, otherTenantList)
}

func TestListInsightsHistory_OrdersNewestValidFromFirst(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	oldValidFrom := time.Now().Add(-48 * time.Hour).Truncate(time.Microsecond)
	newValidFrom := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)

	old, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Old history",
		Column8:     oldValidFrom,
		Column10:    false,
	})
	require.NoError(t, err)
	newer, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Newer history",
		Column8:     newValidFrom,
		Column10:    false,
	})
	require.NoError(t, err)

	history, err := store.ListInsightsHistory(context.Background(), ListInsightsHistoryParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
	})
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, newer.ID, history[0].ID)
	require.Equal(t, old.ID, history[1].ID)
}

func TestGetActiveInsightsByUser_FiltersCurrentAndTenant(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	otherCustomer := createTestCustomer(t, store, otherUser.ID)

	createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumRisk, "Current risk")
	createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumEngagement, "Current engagement")
	createTestCustomerInsight(t, store, otherUser.ID, otherCustomer.ID, InsightTypeEnumRisk, "Other tenant risk")
	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumPaymentBehavior,
		InsightText: "Historical only",
		Column10:    false,
	})
	require.NoError(t, err)

	list, err := store.GetActiveInsightsByUser(context.Background(), user.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	for _, insight := range list {
		require.Equal(t, user.ID, insight.UserID)
		require.True(t, insight.IsCurrent)
	}
}

func TestGetInsightsExpiringSoon_FiltersBoundaryCurrentAndTenant(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	otherCustomer := createTestCustomer(t, store, otherUser.ID)
	soon := time.Now().Add(24 * time.Hour).Truncate(time.Microsecond)
	nearBoundary := time.Now().Add((72 * time.Hour) - time.Minute).Truncate(time.Microsecond)
	later := time.Now().Add((72 * time.Hour) + time.Hour).Truncate(time.Microsecond)

	soonInsight, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Expires tomorrow",
		ValidTo:     &soon,
		Column10:    true,
	})
	require.NoError(t, err)
	boundaryInsight, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumEngagement,
		InsightText: "Expires just inside three days",
		ValidTo:     &nearBoundary,
		Column10:    true,
	})
	require.NoError(t, err)
	_, err = store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumPaymentBehavior,
		InsightText: "Expires after three days",
		ValidTo:     &later,
		Column10:    true,
	})
	require.NoError(t, err)
	_, err = store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      otherUser.ID,
		CustomerID:  otherCustomer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Other tenant expires soon",
		ValidTo:     &soon,
		Column10:    true,
	})
	require.NoError(t, err)

	list, err := store.GetInsightsExpiringSoon(context.Background(), user.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, soonInsight.ID, list[0].ID)
	require.Equal(t, boundaryInsight.ID, list[1].ID)
}

func TestDeleteInsightByID_RemovesOnlyTargetAndIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	target := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumRisk, "Delete me")
	remaining := createTestCustomerInsight(t, store, user.ID, customer.ID, InsightTypeEnumEngagement, "Keep me")

	err := store.DeleteInsightByID(context.Background(), target.ID)
	require.NoError(t, err)
	err = store.DeleteInsightByID(context.Background(), target.ID)
	require.NoError(t, err)

	_, err = store.GetCustomerInsightByID(context.Background(), target.ID)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	fetched, err := store.GetCustomerInsightByID(context.Background(), remaining.ID)
	require.NoError(t, err)
	require.Equal(t, remaining.ID, fetched.ID)
}

func TestCreateCustomerInsight_RejectsMissingUser(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      uuid.New(),
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Missing user",
	})
	require.Error(t, err)
}

func TestCreateCustomerInsight_RejectsMissingCustomer(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  uuid.New(),
		InsightType: InsightTypeEnumRisk,
		InsightText: "Missing customer",
	})
	require.Error(t, err)
}

func TestCreateCustomerInsight_RejectsInvalidInsightType(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnum("unknown"),
		InsightText: "Invalid type",
	})
	require.Error(t, err)
}

func TestCreateCustomerInsight_RejectsInvalidMetadataJSON(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Invalid metadata",
		Column7:     json.RawMessage("{not-json"),
	})
	require.Error(t, err)
}

func TestCreateCustomerInsight_RejectsScoreOutsidePrecision(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	score := "1000.00"

	_, err := store.CreateCustomerInsight(context.Background(), CreateCustomerInsightParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		InsightType: InsightTypeEnumRisk,
		InsightText: "Invalid score",
		Score:       &score,
	})
	require.Error(t, err)
}
