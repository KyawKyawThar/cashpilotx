package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPSender delivers email via SMTP.
// Works with MailHog (no auth) in development and Resend's SMTP relay in production:
//
//	Host: smtp.resend.com, Port: 587, Username: resend, Password: <api-key>
type SMTPSender struct {
	host     string
	port     int
	from     string
	username string // empty = no auth (MailHog)
	password string
}

func NewSMTPSender(host string, port int, from, username, password string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		from:     from,
		username: username,
		password: password,
	}
}

func (s *SMTPSender) SendRegistrationOTP(_ context.Context, to, otp string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: Verify your CashPilotX account",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		registrationOTPEmailBody(otp),
	}, "\r\n")

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	return smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg))
}

func (s *SMTPSender) SendDigest(_ context.Context, to string, data DigestData) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: CashPilotX — Your daily cash recovery summary",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		digestEmailBody(data),
	}, "\r\n")
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	return smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg))
}

func (s *SMTPSender) SendOTP(_ context.Context, to, name, otp string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: Your CashPilotX PIN reset code",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		otpEmailBody(name, otp),
	}, "\r\n")

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	return smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg))
}
