package service

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/token"
	"context"

	"github.com/google/uuid"
)

type AuthServicer interface {
	Register(ctx context.Context, req *dto.RegisterRequest, ipAddress, userAgent string) (*dto.RegisterResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest, ipAddress, userAgent string) (*dto.LoginResponse, error)
	Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest, ipAddress, userAgent string) (*dto.RefreshTokenResponse, error)
	ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error)
	GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error)
	RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error
	ForgotPIN(ctx context.Context, req *dto.ForgotPINRequest) error
	ResetPIN(ctx context.Context, req *dto.ResetPINRequest) error
}

type InvoiceServicer interface {
	CreateInvoice(ctx context.Context, userID uuid.UUID, req *dto.CreateInvoiceRequest) (*dto.CreateInvoiceResponse, error)
	GetInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	ListInvoices(ctx context.Context, userID uuid.UUID, filter *dto.ListInvoicesFilter, pg *dto.Pagination) ([]*dto.InvoiceResponse, error)
	CancelInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	WriteOffInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	MoveCollectionState(ctx context.Context, userID, invoiceID uuid.UUID, req *dto.MoveCollectionStateRequest) (*dto.InvoiceResponse, error)
	GetDashboardStats(ctx context.Context, userID uuid.UUID) (*dto.DashboardResponse, error)
	GetRecoveryQueue(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.RecoveryQueueItemResponse, error)
}

type MessageServicer interface {
	ListMessages(ctx context.Context, invoiceID uuid.UUID) ([]*dto.ChatMessageResponse, error)
	CreateManualMessage(ctx context.Context, userID uuid.UUID, req *dto.CreateManualMessageRequest) (*dto.ChatMessageResponse, error)
	ApproveMessage(ctx context.Context, messageID uuid.UUID) (*dto.ChatMessageResponse, error)
	MarkMessageSent(ctx context.Context, messageID uuid.UUID, req *dto.MarkMessageSentRequest) (*dto.ChatMessageResponse, error)
	GetCopyText(ctx context.Context, messageID uuid.UUID) (*dto.CopyTextResponse, error)
}

type ReminderServicer interface {
	ListReminders(ctx context.Context, invoiceID uuid.UUID) ([]*dto.ReminderResponse, error)
	ListAllReminders(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.ReminderResponse, error)
	CreateReminder(ctx context.Context, userID, invoiceID uuid.UUID, req *dto.CreateReminderRequest) (*dto.ReminderResponse, error)
	MarkReminderSent(ctx context.Context, reminderID uuid.UUID, req *dto.MarkReminderSentRequest) error
	CancelReminder(ctx context.Context, reminderID uuid.UUID) error
}

type AuditServicer interface {
	ListAuditLogs(ctx context.Context, userID uuid.UUID, pg *dto.Pagination, actionType string) ([]*dto.AuditLogResponse, error)
	RevertAuditLog(ctx context.Context, logID, performedBy uuid.UUID) (*dto.AuditLogResponse, error)
}

type PaymentServicer interface {
	RecordPayment(ctx context.Context, userID uuid.UUID, req *dto.RecordPaymentRequest) (*dto.RecordPaymentResponse, error)
	GetPayment(ctx context.Context, userID, paymentID uuid.UUID) (*dto.PaymentResponse, error)
	ListPayments(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.PaymentListItem, error)
	ListPaymentsByInvoice(ctx context.Context, userID, invoiceID uuid.UUID) ([]*dto.PaymentResponse, error)
}

type CustomerServicer interface {
	// Customer CRUD
	CreateCustomer(ctx context.Context, userID uuid.UUID, req *dto.CreateCustomerRequest) (*dto.CustomerResponse, error)
	GetCustomer(ctx context.Context, userID, customerID uuid.UUID) (*dto.CustomerResponse, error)
	ListCustomers(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.CustomerResponse, error)
	SearchCustomers(ctx context.Context, userID uuid.UUID, query string) ([]*dto.CustomerResponse, error)
	UpdateCustomer(ctx context.Context, userID, customerID uuid.UUID, req *dto.UpdateCustomerRequest) (*dto.CustomerResponse, error)
	ArchiveCustomer(ctx context.Context, userID, customerID uuid.UUID) error

	// Channel configs (per-user, not per-customer)
	UpsertChannelConfig(ctx context.Context, userID uuid.UUID, req *dto.UpsertChannelConfigRequest) (*dto.ChannelConfigResponse, error)
	ListChannelConfigs(ctx context.Context, userID uuid.UUID) ([]*dto.ChannelConfigResponse, error)
	GetChannelConfig(ctx context.Context, userID uuid.UUID, channel string) (*dto.ChannelConfigResponse, error)
	DeleteChannelConfig(ctx context.Context, userID uuid.UUID, channel string) error
	SetChannelActive(ctx context.Context, userID uuid.UUID, channel string, active bool) error

	// Customer events timeline
	ListCustomerEvents(ctx context.Context, userID, customerID uuid.UUID, pg *dto.Pagination) ([]*dto.CustomerEventResponse, error)
}
