# 02 · Auth Service

> Identity & tokens. Register/login, PASETO issuance, refresh-token rotation, OAuth2 social login.
> Stack: Go · Chi · PostgreSQL · PASETO v4 · Redis (refresh denylist) · Kafka producer

---

## 1. Responsibility

- Register/login for `customer`, `seller`, `admin`, `driver`.
- Issue **PASETO v4 public** access tokens (15 min) signed with Ed25519.
- Issue + **rotate** refresh tokens (30 d), one-time-use, family-based reuse detection.
- OAuth2 / OIDC social login (Google, Facebook — common in SE Asia).
- Publish `user.registered` for downstream provisioning.
- Expose the Ed25519 **public key** so the gateway verifies tokens locally.

---

## 2. Database schema

```sql
CREATE TYPE user_role AS ENUM ('customer', 'seller', 'admin', 'driver');

CREATE TABLE users (
  id            UUID      PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT      NOT NULL UNIQUE,
  phone         TEXT      UNIQUE,
  password_hash TEXT,                              -- null if OAuth-only
  role          user_role NOT NULL DEFAULT 'customer',
  email_verified BOOLEAN  NOT NULL DEFAULT false,
  status        TEXT      NOT NULL DEFAULT 'active', -- active | suspended | deleted
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oauth_identities (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider   TEXT NOT NULL,            -- 'google' | 'facebook'
  subject    TEXT NOT NULL,            -- provider's user id
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (provider, subject)
);

-- Refresh tokens: one-time-use, rotation with family reuse detection
CREATE TABLE refresh_tokens (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id   UUID NOT NULL,           -- all rotations of one login share a family
  token_hash  TEXT NOT NULL,           -- store hash, never the token
  used        BOOLEAN NOT NULL DEFAULT false,
  expires_at  TIMESTAMPTZ NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_user   ON refresh_tokens(user_id, used);
CREATE INDEX idx_refresh_family ON refresh_tokens(family_id);
```

---

## 3. Token model

```
Access token (PASETO v4 public, Ed25519):
  claims: { sub, role, seller_id?, exp: now+15m, iss: "bazaar-auth" }
  verified LOCALLY by the gateway with the public key — no network call.

Refresh token (opaque, 30d, one-time-use):
  stored hashed; on /refresh the old one is marked used and a new one in the
  same family is issued (rotation). If a USED token is presented again →
  reuse attack → revoke the entire family (force re-login).
```

---

## 4. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | Create account → `user.registered` |
| POST | `/api/v1/auth/login` | — | Returns access + refresh |
| POST | `/api/v1/auth/refresh` | refresh token | Rotate → new access + refresh |
| POST | `/api/v1/auth/logout` | access | Revoke current refresh family |
| GET  | `/api/v1/auth/oauth/{provider}` | — | Begin OAuth2 redirect |
| GET  | `/api/v1/auth/oauth/{provider}/callback` | — | Exchange code → tokens |
| POST | `/api/v1/auth/verify-email` | — | Confirm email token |
| GET  | `/.well-known/bazaar-pubkey` | — | Ed25519 public key (gateway fetches) |

**POST `/auth/register` body:**
```json
{ "email": "a@b.com", "phone": "+66...", "password": "...", "role": "customer" }
```

---

## 5. gRPC (internal)

```
auth.ValidateToken(token)            → claims          (rarely used; gateway verifies locally)
auth.GetUser(user_id)                → {id, email, role, status}
auth.SuspendUser(user_id)            → ok              (admin/abuse flows)
```

---

## 6. Kafka

| Direction | Topic | Payload |
|---|---|---|
| Produce | `user.registered` | `{user_id, role, email, created_at}` |
| Produce | `user.suspended` | `{user_id, reason}` |

`user.registered` → consumed by **user-service** (create profile) and **notification-service** (welcome email).

---

## 7. Transaction functions

```
RegisterUserTx
  1. INSERT users (hash password with argon2id)
  2. (if OAuth) INSERT oauth_identities
  3. emit user.registered via outbox
LoginTx / RotateRefreshTx
  1. verify credentials / verify + mark old refresh used
  2. INSERT new refresh_tokens (same family on rotate)
  3. detect reuse → DELETE family + force re-login
```

---

## 8. Open items

- ⚠ Argon2id params (memory/iterations) to fix in Sprint 0.
- ⚠ Phone/OTP login for SE Asia (many users are phone-first, not email) — consider before launch.
