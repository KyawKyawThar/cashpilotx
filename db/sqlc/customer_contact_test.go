package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestContact(t *testing.T, store *SQLStore, userID, customerID uuid.UUID) CustomerContact {
	t.Helper()

	name := "john"
	meta := []byte(`{"source":"test"}`)

	c, err := store.UpsertCustomerContact(context.Background(), UpsertCustomerContactParams{
		UserID:      userID,
		CustomerID:  customerID,
		Platform:    ContactPlatformLINE,
		ExternalID:  uuid.New().String(),
		DisplayName: &name,
		Meta:        meta,
	})

	require.NoError(t, err)
	require.NotZero(t, c.ID)

	return c
}
func TestUpsertCustomerContact_Insert(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, customer.ID)

	require.Equal(t, user.ID, c.UserID)
	require.Equal(t, customer.ID, c.CustomerID)
	require.True(t, c.IsActive)
}
func TestUpsertCustomerContact_Update(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	externalID := uuid.New().String()
	name1 := "A"
	name2 := "B"

	meta := []byte(`{"v":1}`)

	// first insert
	c1, _ := store.UpsertCustomerContact(context.Background(), UpsertCustomerContactParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		Platform:    ContactPlatformLINE,
		ExternalID:  externalID,
		DisplayName: &name1,
		Meta:        meta,
	})

	// second → should update same row
	c2, err := store.UpsertCustomerContact(context.Background(), UpsertCustomerContactParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		Platform:    ContactPlatformLINE,
		ExternalID:  externalID,
		DisplayName: &name2,
		Meta:        meta,
	})

	require.NoError(t, err)
	require.Equal(t, c1.ID, c2.ID)
	require.Equal(t, name2, *c2.DisplayName)
}
func TestUpsertCustomerContact_Reactivates(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, customer.ID)

	// deactivate
	err := store.DeactivateContact(context.Background(), c.ID)
	require.NoError(t, err)

	// upsert again → should reactivate
	name := "new"
	meta := []byte(`{}`)

	updated, err := store.UpsertCustomerContact(context.Background(), UpsertCustomerContactParams{
		UserID:      user.ID,
		CustomerID:  customer.ID,
		Platform:    c.Platform,
		ExternalID:  c.ExternalID,
		DisplayName: &name,
		Meta:        meta,
	})

	require.NoError(t, err)
	require.True(t, updated.IsActive)
}
func TestGetContactByPlatform_Success(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, customer.ID)

	found, err := store.GetContactByPlatform(context.Background(), GetContactByPlatformParams{
		UserID:     user.ID,
		Platform:   c.Platform,
		ExternalID: c.ExternalID,
	})

	require.NoError(t, err)
	require.Equal(t, c.ID, found.ID)
}
func TestGetContactByPlatform_InactiveFails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, customer.ID)

	_ = store.DeactivateContact(context.Background(), c.ID)

	_, err := store.GetContactByPlatform(context.Background(), GetContactByPlatformParams{
		UserID:     user.ID,
		Platform:   c.Platform,
		ExternalID: c.ExternalID,
	})

	require.Error(t, err)
}
func TestContact_Isolation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	u1 := createTestUser(t, store)
	u2 := createTestUser(t, store)

	cust := createTestCustomer(t, store, u1.ID)
	c := createTestContact(t, store, u1.ID, cust.ID)

	_, err := store.GetContactByPlatform(context.Background(), GetContactByPlatformParams{
		UserID:     u2.ID,
		Platform:   c.Platform,
		ExternalID: c.ExternalID,
	})

	require.Error(t, err)
}
func TestListContactsByCustomer_Order(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	cust := createTestCustomer(t, store, user.ID)

	c1 := createTestContact(t, store, user.ID, cust.ID)
	c2 := createTestContact(t, store, user.ID, cust.ID)

	list, err := store.ListContactsByCustomer(context.Background(), cust.ID)

	require.NoError(t, err)
	require.Len(t, list, 2)
	ids := []uuid.UUID{list[0].ID, list[1].ID}
	require.Contains(t, ids, c1.ID)
	require.Contains(t, ids, c2.ID)
}
func TestListUserContacts_Pagination(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	cust := createTestCustomer(t, store, user.ID)

	for i := 0; i < 5; i++ {
		createTestContact(t, store, user.ID, cust.ID)
	}

	list, err := store.ListUserContacts(context.Background(), ListUserContactsParams{
		UserID: user.ID,
		Limit:  2,
		Offset: 0,
	})

	require.NoError(t, err)
	require.Len(t, list, 2)
}
func TestDeleteContact(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	cust := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, cust.ID)

	err := store.DeleteContact(context.Background(), c.ID)
	require.NoError(t, err)

	_, err = store.GetContactByPlatform(context.Background(), GetContactByPlatformParams{
		UserID:     user.ID,
		Platform:   c.Platform,
		ExternalID: c.ExternalID,
	})

	require.Error(t, err)
}

func TestReassignContact(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	c1 := createTestCustomer(t, store, user.ID)
	c2 := createTestCustomer(t, store, user.ID)

	c := createTestContact(t, store, user.ID, c1.ID)

	updated, err := store.ReassignContact(context.Background(), ReassignContactParams{
		ID:         c.ID,
		CustomerID: c2.ID,
	})

	require.NoError(t, err)
	require.Equal(t, c2.ID, updated.CustomerID)
}
