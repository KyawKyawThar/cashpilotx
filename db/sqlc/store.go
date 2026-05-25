package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Querier
	Ping(ctx context.Context) error

	// Auth / session lifecycle
	RegisterUserTx(ctx context.Context, arg RegisterUserTxParams) (RegisterUserTxResult, error)
	RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (RotateSessionTxResult, error)

	// Customers
	CreateCustomerTx(ctx context.Context, arg CreateCustomerTxParams) (CreateCustomerTxResult, error)

	// AR core
	CreateInvoiceTx(ctx context.Context, arg CreateInvoiceTxParams) (CreateInvoiceTxResult, error)
	RecordPaymentTx(ctx context.Context, arg RecordPaymentTxParams) (RecordPaymentTxResult, error)
	VoidPaymentTx(ctx context.Context, arg VoidPaymentTxParams) (VoidPaymentTxResult, error)
	CancelInvoiceTx(ctx context.Context, arg CancelInvoiceTxParams) (CancelInvoiceTxResult, error)
	ReactivateInvoiceTx(ctx context.Context, arg ReactivateInvoiceTxParams) (ReactivateInvoiceTxResult, error)
	WriteOffInvoiceTx(ctx context.Context, arg WriteOffInvoiceTxParams) (WriteOffInvoiceTxResult, error)
	ReturnInvoiceTx(ctx context.Context, arg ReturnInvoiceTxParams) (ReturnInvoiceTxResult, error)

	// Payments / webhooks
	ProcessWebhookTx(ctx context.Context, arg ProcessWebhookTxParams) (ProcessWebhookTxResult, error)

	// Reminders
	FireReminderTx(ctx context.Context, arg FireReminderTxParams) (FireReminderTxResult, error)

	// Collection lifecycle
	ProcessInboundReplyTx(ctx context.Context, arg ProcessInboundReplyTxParams) (ProcessInboundReplyTxResult, error)
	MoveCollectionStateTx(ctx context.Context, arg MoveCollectionStateTxParams) (MoveCollectionStateTxResult, error)

	// Exchange rates
	UpsertExchangeRateTx(ctx context.Context, arg UpsertExchangeRateTxParams) (UpsertExchangeRateTxResult, error)
	ReplaceExchangeRateTx(ctx context.Context, arg CreateExchangeRateParams) (ExchangeRate, error)

	// Stats / intelligence
	RefreshDailyStatsTx(ctx context.Context, arg RefreshDailyStatsTxParams) (RefreshDailyStatsTxResult, error)
	RefreshRiskProfileTx(ctx context.Context, arg RefreshRiskProfileTxParams) (RefreshRiskProfileTxResult, error)
	RefreshRecoveryQueueTx(ctx context.Context, arg RefreshRecoveryQueueTxParams) (RefreshRecoveryQueueTxResult, error)

	// Delta sync (hand-written — see sync_queries.go)
	SyncInvoices(ctx context.Context, arg SyncInvoicesParams) ([]Invoice, error)
	SyncCustomers(ctx context.Context, arg SyncCustomersParams) ([]Customer, error)

	// Reminder mutations (hand-written — see reminder_update.go)
	UpdateReminder(ctx context.Context, arg UpdateReminderParams) (Reminder, error)

	// User settings (hand-written — see user_settings.go)
	UpdateHomeCurrency(ctx context.Context, arg UpdateHomeCurrencyParams) (User, error)

	// Team user role (hand-written — see update_user_role.go)
	UpdateUserRole(ctx context.Context, arg UpdateUserRoleParams) (User, error)
}

type SQLStore struct {
	*Queries
	connPool *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) *SQLStore {
	return &SQLStore{
		Queries:  New(connPool),
		connPool: connPool,
	}
}

func (s *SQLStore) Ping(ctx context.Context) error {
	return s.connPool.Ping(ctx)
}

// compile-time proof that *SQLStore satisfies Store
var _ Store = (*SQLStore)(nil)
