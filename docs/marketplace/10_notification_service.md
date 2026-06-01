# 10 · Notification Service

> Consumes the whole event bus and turns events into messages. The textbook **Kafka → RabbitMQ** seam:
> Kafka delivers the *fact*, RabbitMQ delivers the *job* (with retry + DLQ).
> Stack: Go · PostgreSQL (log + templates) · Kafka (consumer) · RabbitMQ (send jobs) · Redis · SSE

---

## 1. Responsibility

- Subscribe to all customer/seller-facing lifecycle events.
- Decide channel(s) per user preference: email, SMS, push (FCM), in-app.
- Enqueue **RabbitMQ** send jobs so delivery is retried independently of Kafka consumption.
- Power the in-app notification bell via **SSE**.

---

## 2. Why Kafka in, RabbitMQ out

```
Kafka  order.paid  ─→ notification-service consumer
                       decides: email + push to customer, push to seller
                       ─→ RabbitMQ enqueue: email.send, push.send  (durable, retryable)
                                              │
                       ┌──────────────────────┼─────────────────┐
                       ▼                       ▼                 ▼
                  email worker            sms worker        push worker
                  (SendGrid/SMTP)         (Twilio/local)    (FCM)
                  retry→DLQ               retry→DLQ          retry→DLQ
```

If SendGrid is down, the email job retries from RabbitMQ — Kafka offset is already committed, so the
event stream isn't blocked. Clean separation of "what happened" vs "deliver this message".

---

## 3. Database schema

```sql
CREATE TABLE templates (
  id        TEXT PRIMARY KEY,            -- 'order_paid', 'out_for_delivery', ...
  channel   TEXT NOT NULL,               -- 'email' | 'sms' | 'push'
  locale    TEXT NOT NULL DEFAULT 'th-TH',
  subject   TEXT,
  body      TEXT NOT NULL                -- handlebars-style {{name}}, {{tracking_no}}
);

CREATE TABLE notifications (             -- in-app bell + audit
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL,
  type       TEXT NOT NULL,              -- maps to event_type
  title      TEXT NOT NULL,
  body       TEXT NOT NULL,
  data       JSONB NOT NULL DEFAULT '{}',-- {order_id, tracking_no}
  read       BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notif_user ON notifications(user_id, created_at DESC);

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 4. Event → message map

| Event consumed | Recipient | Message |
|---|---|---|
| `user.registered` | customer | Welcome + verify email |
| `order.created` | customer / seller | "Order placed" / "New order!" |
| `order.paid` | customer | "Payment received, preparing" |
| `inventory.rejected` | customer | "Item out of stock, refunded" |
| `delivery.checkpoint` | customer | "Out for delivery" / "Arriving today" (selected statuses only) |
| `delivery.status.changed`→delivered | customer | "Delivered! Rate your purchase" |
| `order.delivered` | seller | "Order completed" |
| `review.created` | seller | "New review on your product" |
| `review.flagged` | admin | "Review needs moderation" |
| `chat.message.sent` | recipient (if offline) | Push: "New message from {sender}" |
| `payout.processed` | seller | "You've been paid {amount}" |
| `seller.approved` | seller | "Store is live!" |

Not every checkpoint pings the customer — only customer-meaningful ones (out for delivery, delivered, failed).

---

## 5. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/notifications` | any | In-app list |
| POST | `/api/v1/notifications/{id}/read` | any | Mark read |
| GET | `/api/v1/notifications/stream` | any | **SSE** bell updates |
| POST | `/api/v1/me/push-token` | any | Register FCM device token |

---

## 6. Open items

- ⚠ SMS provider per market (Twilio intl vs local SE Asian gateways for delivery + cost).
- ⚠ Rate-limit / dedupe noisy events (don't send 6 push notifications for 6 checkpoints).
- ⚠ Quiet hours / user notification preferences honored before enqueueing.
