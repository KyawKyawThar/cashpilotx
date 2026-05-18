#!/usr/bin/env bash
# seed.sh — populate CashPilotX with 3 test scenarios
# Usage: bash scripts/seed.sh [BASE_URL]
set -euo pipefail

BASE="${1:-http://localhost:8080}"
PHONE="+959700000001"
EMAIL="seed@cashpilotx.dev"
PIN="123456"
NAME="Test Admin"
BUSINESS="CashPilot Seed Co"

echo "==> Base URL: $BASE"

# ─── helpers ──────────────────────────────────────────────────────────────────
uuid() { python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen 2>/dev/null | tr '[:upper:]' '[:lower:]'; }
today() { date +%Y-%m-%d; }
days_ago() { date -v -"${1}"d +%Y-%m-%dT00:00:00Z 2>/dev/null || date -d "-${1} days" +%Y-%m-%dT00:00:00Z; }
days_from_now() { date -v +"${1}"d +%Y-%m-%dT00:00:00Z 2>/dev/null || date -d "+${1} days" +%Y-%m-%dT00:00:00Z; }

post() {
  local url="$1" body="$2"
  shift 2
  curl -sf -X POST "$url" \
    -H "Content-Type: application/json" \
    "$@" \
    -d "$body"
}

auth_post() {
  local url="$1" body="$2"
  shift 2
  curl -sf -X POST "$url" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    "$@" \
    -d "$body"
}

auth_patch() {
  local url="$1" body="$2"
  curl -sf -X PATCH "$url" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Idempotency-Key: $(uuid)" \
    -d "$body"
}

# ─── register (ok to fail if already exists) ──────────────────────────────────
echo ""
echo "==> Registering user..."
post "$BASE/api/v1/auth/register" \
  "{\"phone\":\"$PHONE\",\"email\":\"$EMAIL\",\"pin\":\"$PIN\",\"confirm_pin\":\"$PIN\",\"name\":\"$NAME\",\"business_name\":\"$BUSINESS\",\"home_currency\":\"MMK\"}" \
  || echo "    (already registered — continuing)"

# ─── login ────────────────────────────────────────────────────────────────────
echo "==> Logging in..."
LOGIN=$(post "$BASE/api/v1/auth/login" "{\"phone\":\"$PHONE\",\"pin\":\"$PIN\"}")
TOKEN=$(echo "$LOGIN" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['data']['access_token'])")
echo "    Token: ${TOKEN:0:30}..."

# ─── customers ────────────────────────────────────────────────────────────────
echo ""
echo "==> Creating customers..."

create_customer() {
  local name="$1" phone="$2"
  local body="{\"name\":\"$name\",\"type\":\"customer\",\"phone\":\"$phone\"}"
  local res
  res=$(auth_post "$BASE/api/v1/customers" "$body")
  echo "$res" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])"
}

CUST1=$(create_customer "Ma Hla Hla" "+959777123456")
echo "    Customer 1: Ma Hla Hla → $CUST1"

CUST2=$(create_customer "Nang Phyu Phyu" "+959888234567")
echo "    Customer 2: Nang Phyu Phyu → $CUST2"

CUST3=$(create_customer "Ko Zaw Lin" "+959555345678")
echo "    Customer 3: Ko Zaw Lin → $CUST3"

# ─── invoices ─────────────────────────────────────────────────────────────────
echo ""
echo "==> Creating invoices..."

create_invoice() {
  local cust_id="$1" desc="$2" amount="$3" currency="$4" due_date="$5"
  local body="{\"customer_id\":\"$cust_id\",\"description\":\"$desc\",\"items\":[],\"amount\":$amount,\"currency\":\"$currency\",\"due_date\":\"$due_date\"}"
  local res
  res=$(curl -sf -X POST "$BASE/api/v1/invoices" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Idempotency-Key: $(uuid)" \
    -d "$body")
  echo "$res" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['invoice']['id'])"
}

# Scenario 1: 45 days overdue → will set to no_response
DUE1=$(days_ago 45)
INV1=$(create_invoice "$CUST1" "Wholesale goods – March batch" 588000 "MMK" "$DUE1")
echo "    Invoice 1: K588,000 due $DUE1 → $INV1"

# Scenario 2: 24 days overdue, replied today → will set to negotiating
DUE2=$(days_ago 24)
INV2=$(create_invoice "$CUST2" "Retail supply – April order" 320000 "MMK" "$DUE2")
echo "    Invoice 2: K320,000 due $DUE2 → $INV2"

# Scenario 3: due in 4 days, sent reminder → will set to reminded
DUE3=$(days_from_now 4)
INV3=$(create_invoice "$CUST3" "Service fee – May" 150000 "MMK" "$DUE3")
echo "    Invoice 3: K150,000 due $DUE3 → $INV3"

# ─── collection states ────────────────────────────────────────────────────────
echo ""
echo "==> Setting collection states..."

# INV1: reminded → no_response (must go through reminded first)
echo "    INV1: pending → reminded"
auth_patch "$BASE/api/v1/invoices/$INV1/collection-state" \
  '{"state":"reminded","note":"First reminder sent via SMS"}' > /dev/null

echo "    INV1: reminded → no_response"
auth_patch "$BASE/api/v1/invoices/$INV1/collection-state" \
  '{"state":"no_response","note":"No reply after 3 attempts — escalating"}' > /dev/null

# INV2: reminded → negotiating
echo "    INV2: pending → reminded"
auth_patch "$BASE/api/v1/invoices/$INV2/collection-state" \
  '{"state":"reminded","note":"Payment reminder sent"}' > /dev/null

echo "    INV2: reminded → negotiating"
auth_patch "$BASE/api/v1/invoices/$INV2/collection-state" \
  '{"state":"negotiating","note":"Customer replied this morning, discussing payment date"}' > /dev/null

# INV3: reminded
echo "    INV3: pending → reminded"
auth_patch "$BASE/api/v1/invoices/$INV3/collection-state" \
  '{"state":"reminded","note":"Invoice sent, awaiting first response"}' > /dev/null

# ─── payments (generates payment_recorded audit logs) ─────────────────────────
echo ""
echo "==> Recording payments..."

record_payment() {
  local inv_id="$1" cust_id="$2" amount="$3" currency="$4" method="$5"
  curl -sf -X POST "$BASE/api/v1/payments" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Idempotency-Key: $(uuid)" \
    -d "{\"invoice_id\":\"$inv_id\",\"customer_id\":\"$cust_id\",\"amount\":$amount,\"method\":\"$method\",\"paid_on\":\"$(days_ago 1)\"}" \
    > /dev/null && echo "    ✓ $method $amount $currency on invoice $inv_id" || echo "    (payment already recorded or skipped)"
}

# Partial payment on INV1
record_payment "$INV1" "$CUST1" 200000 "MMK" "cash"

# Full payment on INV3
record_payment "$INV3" "$CUST3" 150000 "MMK" "transfer"

# ─── manual messages (generates message_copied / message_marked_sent logs) ────
echo ""
echo "==> Creating manual messages..."

create_manual_message() {
  local inv_id="$1" body_text="$2"
  local res
  res=$(curl -sf -X POST "$BASE/api/v1/messages/manual" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"invoice_id\":\"$inv_id\",\"body\":\"$body_text\",\"channel\":\"whatsapp\"}" 2>/dev/null) || return 0
  MSG_ID=$(echo "$res" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['data']['id'])" 2>/dev/null || true)
  echo "    Message created: $MSG_ID"
  echo "$MSG_ID"
}

MSG1=$(create_manual_message "$INV1" "Dear Ma Hla Hla, your invoice of K588,000 is now 45 days overdue. Please advise on payment timeline.")
MSG2=$(create_manual_message "$INV2" "Hi Nang Phyu Phyu, following up on our discussion — can you confirm the payment date?")

# Mark message as sent (generates message_marked_sent audit log)
if [ -n "$MSG1" ]; then
  curl -sf -X POST "$BASE/api/v1/messages/$MSG1/mark-sent" \
    -H "Authorization: Bearer $TOKEN" > /dev/null \
    && echo "    ✓ MSG1 marked sent" || echo "    (mark-sent skipped)"
fi

# ─── reminders ────────────────────────────────────────────────────────────────
echo ""
echo "==> Scheduling reminders..."

schedule_reminder() {
  local inv_id="$1" channel="$2" tone="$3" days_out="$4"
  local sched_at
  sched_at=$(date -v +"${days_out}"d +%Y-%m-%dT09:00:00Z 2>/dev/null || date -d "+${days_out} days" +%Y-%m-%dT09:00:00Z)
  curl -sf -X POST "$BASE/api/v1/invoices/$inv_id/reminders" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"channel\":\"$channel\",\"tone\":\"$tone\",\"scheduled_at\":\"$sched_at\"}" > /dev/null \
    && echo "    ✓ Reminder ($tone $channel) scheduled for invoice $inv_id in ${days_out}d" \
    || echo "    (reminder skipped)"
}

schedule_reminder "$INV1" "sms"      "firm"    3
schedule_reminder "$INV2" "whatsapp" "polite"  5

# ─── done ─────────────────────────────────────────────────────────────────────
echo ""
echo "✓ Seed complete!"
echo ""
echo "  Scenario 1 → $INV1  (Ma Hla Hla, K588,000, no_response, 45d overdue)"
echo "  Scenario 2 → $INV2  (Nang Phyu Phyu, K320,000, negotiating, 24d overdue)"
echo "  Scenario 3 → $INV3  (Ko Zaw Lin, K150,000, reminded, due in 4d)"
echo ""
echo "  Activity log entries generated:"
echo "    • collection_state_changed (×5 state transitions above)"
echo "    • payment_recorded         (×2 payments)"
echo "    • message_marked_sent      (×1 manual mark-sent)"
echo "    • reminder_scheduled       (×2 reminders)"
echo ""
echo "  Login: phone=$PHONE  pin=$PIN"
echo ""
echo "  To add more sample audit logs directly:"
echo "    psql \$DATABASE_URL -f scripts/seed_audit_logs.sql"
