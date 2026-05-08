// Package email provides an abstraction for sending transactional emails.
// Use SMTPSender (MailHog) in development and ResendSender in production.
package email

import "context"

// Sender is the single interface all email backends must satisfy.
type Sender interface {
	SendOTP(ctx context.Context, to, name, otp string) error
}
