# 14 · Chat Service

> Realtime customer ↔ seller (and ↔ support) chat. WebSocket front, Redis pub/sub fan-out,
> Postgres history. Shares the `shared/websocket` hub with delivery-service.
> Stack: Go · WebSocket · Redis (pub/sub + presence) · PostgreSQL · Kafka (producer + consumer)

---

## 1. Responsibility

- Realtime messaging between customer and seller (pre-purchase questions) and customer ↔ support.
- Persist message history (searchable) in Postgres.
- Online presence + typing indicators via Redis.
- Emit `chat.message.sent` so notification-service can push to offline recipients.
- Auto-open a support room when `order.created` (optional, for order issues).

---

## 2. Scaling WebSocket across instances (Redis pub/sub)

```
Customer socket on instance A      Seller socket on instance B
        │                                   │
        └──── both subscribe to ────────────┘
              Redis channel: room:{room_id}

Send message → persist in PG → PUBLISH room:{room_id} → every instance with a
subscriber in that room delivers to its local sockets. (Same pattern as delivery live map.)
```

This is why chat + delivery share `shared/websocket` — identical hub/room/Redis-bridge mechanics.

---

## 3. Database schema

```sql
CREATE TYPE room_type AS ENUM ('customer_seller','customer_support');

CREATE TABLE rooms (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type        room_type NOT NULL,
  customer_id UUID NOT NULL,
  seller_id   UUID,                        -- null for support rooms
  order_id    UUID,                        -- optional context
  last_message_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, customer_id, seller_id, order_id)
);
CREATE INDEX idx_rooms_customer ON rooms(customer_id, last_message_at DESC);
CREATE INDEX idx_rooms_seller   ON rooms(seller_id, last_message_at DESC);

CREATE TABLE messages (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id     UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  sender_id   UUID NOT NULL,
  sender_role TEXT NOT NULL,               -- 'customer' | 'seller' | 'support'
  body        TEXT,
  attachments JSONB NOT NULL DEFAULT '[]', -- file-service URLs
  read_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_room ON messages(room_id, created_at);

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

Presence (`online:{user_id}`) and typing (`typing:{room_id}`) live in Redis with short TTLs.

---

## 4. WebSocket protocol

```
WSS /api/v1/chat   (auth via PASETO in connect)
  client → { type:"join", room_id }
  client → { type:"message", room_id, body, attachments? }
  client → { type:"typing", room_id }
  client → { type:"read", room_id, up_to_message_id }

  server → { type:"message", message }
  server → { type:"typing", user_id }
  server → { type:"presence", user_id, online }
```

---

## 5. REST routes (history + room list)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/chat/rooms` | customer/seller | Room list (latest message preview) |
| POST | `/api/v1/chat/rooms` | customer | Start chat with a seller |
| GET | `/api/v1/chat/rooms/{id}/messages` | participant | History (paginated) |

---

## 6. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Produce | `chat.message.sent` | key=room_id → notification (push if recipient offline) |
| Consume | `order.created` | optionally auto-open a support room for the order |

Presence (online/offline) decides whether notification-service actually pushes — if the recipient's
socket is connected, no push is sent.

---

## 7. Open items

- ⚠ Claude-powered auto-reply / FAQ bot as first responder (ties to your CashPilotX Claude usage) — post-MVP.
- ⚠ Message retention + search (Postgres full-text vs reuse Elasticsearch).
- ⚠ Abuse: rate-limit messages, block list per user.
