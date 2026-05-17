// Package notify sends Web Push notifications to subscribed browsers.
package notify

import (
	"context"
	"encoding/json"
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// PushPayload is the JSON body delivered to the browser's push handler.
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
}

// Subscription mirrors the browser PushSubscription object stored in the DB.
type Subscription struct {
	Endpoint string
	P256DH   string
	Auth     string
}

// Sender sends Web Push notifications using VAPID.
type Sender struct {
	vapidPublicKey  string
	vapidPrivateKey string
	vapidEmail      string
}

// NewSender creates a push Sender. Call once at startup with your VAPID keys.
// Generate keys once with: webpush.GenerateVAPIDKeys()
func NewSender(publicKey, privateKey, email string) *Sender {
	return &Sender{
		vapidPublicKey:  publicKey,
		vapidPrivateKey: privateKey,
		vapidEmail:      email,
	}
}

// Send delivers a push notification to a single subscription.
// A 410 Gone response from the push service means the subscription has expired
// and should be deleted from the database — the caller is responsible for this.
func (s *Sender) Send(_ context.Context, sub Subscription, payload PushPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("push: marshal payload: %w", err)
	}

	resp, err := webpush.SendNotification(body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256DH,
			Auth:   sub.Auth,
		},
	}, &webpush.Options{
		VAPIDPublicKey:  s.vapidPublicKey,
		VAPIDPrivateKey: s.vapidPrivateKey,
		Subscriber:      s.vapidEmail,
		TTL:             86400, // 24 h
	})
	if err != nil {
		return fmt.Errorf("push: send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 410 {
		return ErrSubscriptionGone
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("push: push service returned %d", resp.StatusCode)
	}
	return nil
}

// ErrSubscriptionGone is returned when the push service reports the
// subscription endpoint is no longer valid (HTTP 410). Callers should
// delete the subscription from the database.
var ErrSubscriptionGone = fmt.Errorf("push: subscription expired (410 Gone)")
