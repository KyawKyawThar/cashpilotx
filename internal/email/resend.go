package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const resendAPIURL = "https://api.resend.com/emails"

// ResendSender delivers email via Resend's HTTP API.
// Unlike the SMTP relay, this uses port 443 which is never blocked by cloud providers.
type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
}

func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{},
	}
}

func (s *ResendSender) SendRegistrationOTP(ctx context.Context, to, otp string) error {
	return s.send(ctx, to, "Verify your CashPilotX account", registrationOTPEmailBody(otp))
}

func (s *ResendSender) SendDigest(ctx context.Context, to string, data DigestData) error {
	return s.send(ctx, to, "CashPilotX — Your daily cash recovery summary", digestEmailBody(data))
}

func (s *ResendSender) SendOTP(ctx context.Context, to, name, otp string) error {
	return s.send(ctx, to, "Your CashPilotX PIN reset code", otpEmailBody(name, otp))
}

func (s *ResendSender) send(ctx context.Context, to, subject, html string) error {
	payload, err := json.Marshal(map[string]any{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	})
	if err != nil {
		return fmt.Errorf("resend: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("resend: API error %d (could not read body: %w)", resp.StatusCode, err)
		}
		return fmt.Errorf("resend: API error %d: %s", resp.StatusCode, body)
	}
	return nil
}
