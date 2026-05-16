// Package email provides an abstraction for sending transactional emails.
// Use SMTPSender (MailHog) in development and ResendSender in production.
package email

import "context"

// DigestData holds the summary numbers for a user's daily digest email.
type DigestData struct {
	Name          string
	Currency      string
	OverdueCount  int
	OverdueTotal  int64 // in minor units (e.g. cents/pyas)
	DueTodayCount int
	DueTodayTotal int64
	AppURL        string
}

// Sender is the single interface all email backends must satisfy.
type Sender interface {
	SendOTP(ctx context.Context, to, name, otp string) error
	SendRegistrationOTP(ctx context.Context, to, otp string) error
	SendDigest(ctx context.Context, to string, data DigestData) error
}
