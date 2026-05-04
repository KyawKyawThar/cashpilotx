package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// newTestStore is for parallel CRUD tests.
// All queries run inside a transaction that is rolled back on t.Cleanup,
// so tests never touch persistent data and can run with t.Parallel().
//
// Do NOT use this for *Tx methods or concurrency tests — those open their own
// transactions via connPool and will not be covered by the rollback.
// Use the package-level testStore for those cases.
func newTestStore(t *testing.T) *SQLStore {
	t.Helper()

	tx, err := testDB.Begin(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		// Silently ignore ErrTxClosed: a *Tx method already committed its own
		// inner transaction, so there is nothing left to roll back here.
		_ = tx.Rollback(context.Background())
	})

	return &SQLStore{
		Queries:  New(tx),
		connPool: testDB,
	}
}
func cleanupUser(t *testing.T, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

func toJSON(t *testing.T, v interface{}) *json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	r := json.RawMessage(b)
	return &r
}
