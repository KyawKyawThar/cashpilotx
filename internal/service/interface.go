package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/token"
	"context"
	"time"

	"github.com/google/uuid"
)

type AuthServicer interface {
	RequestEmailVerification(ctx context.Context, email string) error
	Register(ctx context.Context, req *dto.RegisterRequest, ipAddress, userAgent string) (*dto.RegisterResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest, ipAddress, userAgent string) (*dto.LoginResponse, error)
	Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest, ipAddress, userAgent string) (*dto.RefreshTokenResponse, error)
	ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error)
	GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error)
	RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error
	ForgotPIN(ctx context.Context, req *dto.ForgotPINRequest) error
	ResetPIN(ctx context.Context, req *dto.ResetPINRequest) error
	GetMe(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)
	UpdateHomeCurrency(ctx context.Context, userID uuid.UUID, currency string) (*dto.UserResponse, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	ChangePIN(ctx context.Context, userID uuid.UUID, req *dto.ChangePINRequest) (*dto.ChangePINResponse, error)
}

type InvoiceServicer interface {
	CreateInvoice(ctx context.Context, userID uuid.UUID, req *dto.CreateInvoiceRequest) (*dto.CreateInvoiceResponse, error)
	GetInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	ListInvoices(ctx context.Context, userID uuid.UUID, filter *dto.ListInvoicesFilter, pg *dto.Pagination) ([]*dto.InvoiceResponse, error)
	CancelInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	ReactivateInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	WriteOffInvoice(ctx context.Context, userID, invoiceID uuid.UUID) (*dto.InvoiceResponse, error)
	MoveCollectionState(ctx context.Context, userID, invoiceID uuid.UUID, req *dto.MoveCollectionStateRequest) (*dto.InvoiceResponse, error)
	UpdateInvoiceTracking(ctx context.Context, userID, invoiceID uuid.UUID, req *dto.UpdateInvoiceTrackingRequest) (*dto.InvoiceResponse, error)
	GetDashboardStats(ctx context.Context, userID uuid.UUID) (*dto.DashboardResponse, error)
	GetCollectedSummary(ctx context.Context, userID uuid.UUID, from, to time.Time) (*dto.CollectedSummaryResponse, error)
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
	UpdateReminder(ctx context.Context, reminderID uuid.UUID, req *dto.UpdateReminderRequest) (*dto.ReminderResponse, error)
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
	VoidPayment(ctx context.Context, userID, paymentID uuid.UUID, customerID uuid.UUID) (*dto.PaymentResponse, error)
}

type SyncServicer interface {
	GetSyncData(ctx context.Context, userID uuid.UUID, since time.Time) (*dto.SyncResponse, error)
}

type TeamServicer interface {
	// Teams
	CreateTeam(ctx context.Context, req *dto.CreateTeamRequest) (*dto.TeamResponse, error)
	ListTeams(ctx context.Context) ([]*dto.TeamResponse, error)

	// Team users
	CreateTeamUser(ctx context.Context, callerID uuid.UUID, callerRole db.UserRole, orgOwnerID uuid.UUID, req *dto.CreateTeamUserRequest) (*dto.TeamMemberResponse, error)
	ListTeamMembers(ctx context.Context, callerRole db.UserRole, callerTeamID *uuid.UUID, orgOwnerID uuid.UUID, activeOnly bool) ([]*dto.TeamMemberResponse, error)
	UpdateTeamMember(ctx context.Context, callerRole db.UserRole, memberID uuid.UUID, req *dto.UpdateTeamUserRequest) (*dto.TeamMemberResponse, error)
	DeactivateMember(ctx context.Context, callerRole db.UserRole, memberID uuid.UUID) error
	ResetMemberPIN(ctx context.Context, callerRole db.UserRole, memberID uuid.UUID, req *dto.AdminResetPINRequest) error

	// Permission grants (super_admin only)
	GrantPermission(ctx context.Context, callerID uuid.UUID, targetUserID uuid.UUID, req *dto.GrantPermissionRequest) (*dto.PermissionGrantResponse, error)
	ListGrants(ctx context.Context, targetUserID uuid.UUID) ([]*dto.PermissionGrantResponse, error)
	RevokeGrant(ctx context.Context, targetUserID uuid.UUID, grantID uuid.UUID) error
}

type ProductServicer interface {
	ListProducts(ctx context.Context, orgOwnerID uuid.UUID, role db.UserRole, activeOnly bool) ([]*dto.ProductResponse, error)
	CreateProduct(ctx context.Context, callerID, orgOwnerID uuid.UUID, req *dto.CreateProductRequest) (*dto.ProductResponse, error)
	UpdateProduct(ctx context.Context, productID, orgOwnerID uuid.UUID, req *dto.UpdateProductRequest) (*dto.ProductResponse, error)
	DeactivateProduct(ctx context.Context, productID, orgOwnerID uuid.UUID) error
}

type CustomerServicer interface {
	// Customer CRUD
	CreateCustomer(ctx context.Context, userID uuid.UUID, assignedTo *uuid.UUID, req *dto.CreateCustomerRequest) (*dto.CustomerResponse, error)
	GetCustomer(ctx context.Context, userID, customerID uuid.UUID) (*dto.CustomerResponse, error)
	ListCustomers(ctx context.Context, userID uuid.UUID, filter *dto.ListCustomersFilter, pg *dto.Pagination) ([]*dto.CustomerResponse, error)
	ListAllCustomers(ctx context.Context, userID uuid.UUID, pg *dto.Pagination) ([]*dto.CustomerResponse, error)
	SearchCustomers(ctx context.Context, userID uuid.UUID, query string) ([]*dto.CustomerResponse, error)
	UpdateCustomer(ctx context.Context, userID, customerID uuid.UUID, req *dto.UpdateCustomerRequest) (*dto.CustomerResponse, error)
	ArchiveCustomer(ctx context.Context, userID, customerID uuid.UUID) error
	ReactivateCustomer(ctx context.Context, userID, customerID uuid.UUID) error

	// Channel configs (per-user, not per-customer)
	UpsertChannelConfig(ctx context.Context, userID uuid.UUID, req *dto.UpsertChannelConfigRequest) (*dto.ChannelConfigResponse, error)
	ListChannelConfigs(ctx context.Context, userID uuid.UUID) ([]*dto.ChannelConfigResponse, error)
	GetChannelConfig(ctx context.Context, userID uuid.UUID, channel string) (*dto.ChannelConfigResponse, error)
	DeleteChannelConfig(ctx context.Context, userID uuid.UUID, channel string) error
	SetChannelActive(ctx context.Context, userID uuid.UUID, channel string, active bool) error

	// Customer events timeline
	ListCustomerEvents(ctx context.Context, userID, customerID uuid.UUID, pg *dto.Pagination) ([]*dto.CustomerEventResponse, error)
}
