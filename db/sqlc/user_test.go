package db

import (
	"cashpilotx/utils"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestUser(t *testing.T, store *SQLStore) User {
	t.Helper()

	if store == nil {
		store = newTestStore(t)
	}

	bn := utils.RandomString(8)
	local := "en"
	user, err := store.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(9),
		PinHash:      "test_hash",
		Name:         utils.RandomString(6),
		BusinessName: &bn,
		Locale:       &local,
		Plan:         "pro",
		HomeCurrency: "USD",
	})
	require.NoError(t, err)
	require.NotZero(t, user.ID)
	return user
}

func TestCreateUser_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	require.NotZero(t, user.ID)
	require.NotEmpty(t, user.Phone)
	require.Equal(t, "pro", user.Plan)
	require.Equal(t, "USD", user.HomeCurrency)

	require.Equal(t, int32(0), user.StreakDays)
	require.Equal(t, int32(0), user.StreakBest)

	require.True(t, user.IsActive)
	require.False(t, user.CreatedAt.IsZero())
	require.False(t, user.UpdatedAt.IsZero())
}

func TestCreateUser_DuplicatePhone_Fails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	phone := utils.RandomPhone(9)

	bn := utils.RandomString(8)
	local := "en"

	_, err := store.CreateUser(context.Background(), CreateUserParams{
		Phone:        phone,
		PinHash:      "test_hash",
		Name:         utils.RandomString(6),
		BusinessName: &bn,
		Locale:       &local,
		Plan:         "pro",
		HomeCurrency: "USD",
	})
	require.NoError(t, err)

	_, err = store.CreateUser(context.Background(), CreateUserParams{
		Phone:        phone,
		PinHash:      "test_hash",
		Name:         utils.RandomString(6),
		BusinessName: &bn,
		Locale:       &local,
		Plan:         "pro",
		HomeCurrency: "USD",
	})
	require.Error(t, err, "duplicate phone must be rejected")
}

func TestGetUserByID_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	fetched, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)

	require.Equal(t, user.ID, fetched.ID)
	require.Equal(t, user.Phone, fetched.Phone)
	require.Equal(t, user.Name, fetched.Name)
}

func TestGetUserByID_NotFound_Fails(t *testing.T) {
	t.Parallel()
	_, err := testStore.GetUserByID(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestGetUserByPhone_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	fetched, err := store.GetUserByPhone(context.Background(), user.Phone)
	require.NoError(t, err)
	require.Equal(t, user.ID, fetched.ID)
}

func TestGetUserByPhone_NotFound_Fails(t *testing.T) {
	t.Parallel()
	_, err := testStore.GetUserByPhone(context.Background(), "not_exist")
	require.Error(t, err)
}

func TestUpdateUser_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	name := "Updated Name"
	biz := "Updated Biz"
	locale := "th"
	updated, err := store.UpdateUserProfile(context.Background(), UpdateUserProfileParams{
		ID:           user.ID,
		Name:         &name,
		BusinessName: &biz,
		Locale:       &locale,
	})
	require.NoError(t, err)

	require.Equal(t, "Updated Name", updated.Name)
	require.Equal(t, "Updated Biz", *updated.BusinessName)
	require.Equal(t, "th", *updated.Locale)

	// unchanged fields
	require.Equal(t, user.Phone, updated.Phone)
	require.Equal(t, user.PinHash, updated.PinHash)
	require.Equal(t, user.Plan, updated.Plan)

}

func TestUpdateUserPlan_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	updated, err := store.UpdateUserPlan(context.Background(), UpdateUserPlanParams{
		ID:   user.ID,
		Plan: "pro",
	})
	require.NoError(t, err)

	require.Equal(t, "pro", updated.Plan)
}

func TestUpdateUserStreak_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	updated, err := store.UpdateUserStreak(context.Background(), UpdateUserStreakParams{
		ID:         user.ID,
		StreakDays: 5,
	})
	require.NoError(t, err)
	require.Equal(t, int32(5), updated.StreakDays)

}
func TestUpdateUserStreak_KeepBest(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	// first increase
	_, _ = store.UpdateUserStreak(context.Background(), UpdateUserStreakParams{
		ID:         user.ID,
		StreakDays: 10,
	})

	// then lower
	updated, err := store.UpdateUserStreak(context.Background(), UpdateUserStreakParams{
		ID:         user.ID,
		StreakDays: 3,
	})

	require.NoError(t, err)
	require.Equal(t, int32(3), updated.StreakDays)
	require.Equal(t, int32(10), updated.StreakBest) // must not decrease
}
func TestUpdateOnboardingStep_Success(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	updated, err := store.UpdateOnboardingStep(context.Background(), UpdateOnboardingStepParams{
		ID:             user.ID,
		OnboardingStep: 3,
	})

	require.NoError(t, err)
	require.Equal(t, int16(3), updated.OnboardingStep)
	require.NotNil(t, updated.OnboardingDoneAt)
	require.False(t, updated.OnboardingDoneAt.IsZero())
}

func TestDeactivateUser(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	err := store.DeactivateUser(context.Background(), user.ID)
	require.NoError(t, err)

	// should not be found anymore
	_, err = store.GetUserByID(context.Background(), user.ID)
	require.Error(t, err)

	_, err = store.GetUserByPhone(context.Background(), user.Phone)
	require.Error(t, err)
}
func TestUpdateUserProfile_UpdatesTimestamp(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	initial := user.UpdatedAt
	time.Sleep(5 * time.Millisecond)

	name := "Changed"
	updated, err := store.UpdateUserProfile(context.Background(), UpdateUserProfileParams{
		ID:   user.ID,
		Name: &name,
	})

	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(initial))
}
func TestCreateUser_InvalidPhone_Fails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, err := store.CreateUser(context.Background(), CreateUserParams{
		Phone:        "twewtew",
		PinHash:      "hash",
		Name:         "test",
		Plan:         "basic",
		HomeCurrency: "USD",
	})

	require.Error(t, err)
}

func TestCreateUser_PhoneBoundary(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// min valid (8 digits)
	u1, err := store.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(8),
		PinHash:      "hash",
		Name:         "min",
		Plan:         "free",
		HomeCurrency: "USD",
	})
	require.NoError(t, err)
	require.NotZero(t, u1.ID)

	// max valid (15 digits)
	u2, err := store.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(15),
		PinHash:      "hash",
		Name:         "max",
		Plan:         "free",
		HomeCurrency: "USD",
	})
	require.NoError(t, err)
	require.NotZero(t, u2.ID)

	// below min (7 digits) → FAIL
	_, err = store.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(7),
		PinHash:      "hash",
		Name:         "fail",
		Plan:         "free",
		HomeCurrency: "USD",
	})
	require.Error(t, err)

	// above max (16 digits) → FAIL
	_, err = store.CreateUser(context.Background(), CreateUserParams{
		Phone:        utils.RandomPhone(16),
		PinHash:      "hash",
		Name:         "fail",
		Plan:         "free",
		HomeCurrency: "USD",
	})
	require.Error(t, err)
}
func TestUpdateUserProfile_InvalidID_Fails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	name := "x"
	_, err := store.UpdateUserProfile(context.Background(), UpdateUserProfileParams{
		ID:   uuid.New(),
		Name: &name,
	})

	require.Error(t, err)
}
func TestUpdateUserStreak_Zero(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	updated, err := store.UpdateUserStreak(context.Background(), UpdateUserStreakParams{
		ID:         user.ID,
		StreakDays: 0,
	})

	require.NoError(t, err)
	require.Equal(t, int32(0), updated.StreakDays)
}
func TestDeactivateUser_Idempotent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	err := store.DeactivateUser(context.Background(), user.ID)
	require.NoError(t, err)

	// second call should NOT crash
	err = store.DeactivateUser(context.Background(), user.ID)
	require.NoError(t, err)
}
func TestUpdateOnboardingStep_NotFinal(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	updated, err := store.UpdateOnboardingStep(context.Background(), UpdateOnboardingStepParams{
		ID:             user.ID,
		OnboardingStep: 2,
	})

	require.NoError(t, err)
	require.Nil(t, updated.OnboardingDoneAt) // not completed yet
}
