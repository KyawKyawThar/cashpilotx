package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestDailyStats(t *testing.T, store *SQLStore, userID uuid.UUID, date time.Time) DailyStat {
	t.Helper()

	stat, err := store.UpsertDailyStats(context.Background(), UpsertDailyStatsParams{
		UserID:           userID,
		Date:             date,
		TotalOutstanding: 1000,
		TotalCollected:   200,
		InvoicesUnpaid:   5,
		InvoicesPaid:     2,
		MessagesSent:     3,
	})

	require.NoError(t, err)
	return stat
}
func TestUpsertDailyStats_Insert(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	date := time.Now()

	stat := createTestDailyStats(t, store, user.ID, date)

	require.Equal(t, int64(1000), stat.TotalOutstanding)
	require.Equal(t, int32(5), stat.InvoicesUnpaid)
}
func TestUpsertDailyStats_Update(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	date := time.Now()

	createTestDailyStats(t, store, user.ID, date)

	updated, err := store.UpsertDailyStats(context.Background(), UpsertDailyStatsParams{
		UserID:           user.ID,
		Date:             date,
		TotalOutstanding: 2000,
		TotalCollected:   500,
		InvoicesUnpaid:   3,
		InvoicesPaid:     4,
		MessagesSent:     10,
	})

	require.NoError(t, err)

	require.Equal(t, int64(2000), updated.TotalOutstanding)
	require.Equal(t, int32(10), updated.MessagesSent)
}
func TestUpsertDailyStats_Fail_NegativeValues(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	_, err := store.UpsertDailyStats(context.Background(), UpsertDailyStatsParams{
		UserID:           user.ID,
		Date:             time.Now(),
		TotalOutstanding: -100, // ❌ invalid
	})

	require.Error(t, err)
}
func TestGetDailyStats_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	date := time.Now()

	expected := createTestDailyStats(t, store, user.ID, date)

	res, err := store.GetDailyStats(context.Background(), GetDailyStatsParams{
		UserID: user.ID,
		Date:   date,
	})

	require.NoError(t, err)
	require.Equal(t, expected.ID, res.ID)
}
func TestGetDailyStats_NotFound(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	_, err := store.GetDailyStats(context.Background(), GetDailyStatsParams{
		UserID: user.ID,
		Date:   time.Now(),
	})

	require.Error(t, err)
}
func TestGetLatestDailyStats(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	d1 := time.Now().AddDate(0, 0, -1)
	d2 := time.Now()

	createTestDailyStats(t, store, user.ID, d1)
	latest := createTestDailyStats(t, store, user.ID, d2)

	res, err := store.GetLatestDailyStats(context.Background(), user.ID)

	require.NoError(t, err)
	require.Equal(t, latest.Date, res.Date)
}
func TestIncrementMessagesSent(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	date := time.Now()

	createTestDailyStats(t, store, user.ID, date)

	err := store.IncrementMessagesSent(context.Background(), IncrementMessagesSentParams{
		UserID:       user.ID,
		Date:         date,
		MessagesSent: 5,
	})

	require.NoError(t, err)

	res, _ := store.GetDailyStats(context.Background(), GetDailyStatsParams{
		UserID: user.ID,
		Date:   date,
	})

	require.Equal(t, int32(8), res.MessagesSent) // 3 + 5
}
func TestIncrementMessagesSent_NoRow(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	err := store.IncrementMessagesSent(context.Background(), IncrementMessagesSentParams{
		UserID:       user.ID,
		Date:         time.Now(),
		MessagesSent: 5,
	})

	// UPDATE 0 rows → no error
	require.NoError(t, err)
}
func TestDeleteStatsByDateRange(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	d1 := time.Now().AddDate(0, 0, -2)
	d2 := time.Now().AddDate(0, 0, -1)

	createTestDailyStats(t, store, user.ID, d1)
	createTestDailyStats(t, store, user.ID, d2)

	err := store.DeleteStatsByDateRange(context.Background(), DeleteStatsByDateRangeParams{
		UserID: user.ID,
		Date:   d1,
		Date_2: d2,
	})

	require.NoError(t, err)

	_, err = store.GetDailyStats(context.Background(), GetDailyStatsParams{
		UserID: user.ID,
		Date:   d1,
	})

	require.Error(t, err)
}
func TestDailyStats_UniqueUserDate(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	date := time.Now()

	first := createTestDailyStats(t, store, user.ID, date)

	second, err := store.UpsertDailyStats(context.Background(), UpsertDailyStatsParams{
		UserID: user.ID,
		Date:   date,
	})

	require.NoError(t, err)

	// should update same row
	require.Equal(t, first.ID, second.ID)
}
