# 09 · Delivery Service

> Your own parcel tracking — the SPX-style timeline + live map you showed in the screenshots.
> The most Kafka-intensive service: checkpoint events, a GPS firehose, route building, and a
> WebSocket hub pushing live location to the customer's map.
> Stack: Go · Chi · PostgreSQL · Kafka (heavy) · WebSocket · Redis (pub/sub + presence)

---

## 1. Responsibility

- On `suborder.paid` → create a **shipment** with a `tracking_no` (e.g. `TH268816827517W`).
- Ingest **checkpoint scans** from the driver app → ordered timeline (the gray event list).
- Ingest **GPS heartbeats** from drivers → build the route **polyline** (the red line on the map).
- Push **live location** + timeline updates to the customer over **WebSocket**.
- Emit `delivery.checkpoint`, `delivery.location`, `delivery.status.changed`.

This service maps directly to your screenshots:
```
"Delivered on 28 May" + Shipped/Out for Delivery/Delivered bar   → status + checkpoints
TH268816827517W                                                  → tracking_no
Timeline (picked up → sorting → last mile → delivered)           → checkpoint events
Red route line + moving pin on the map                           → location points + WS
```

---

## 2. Checkpoint status model (from your screenshots)

```go
type CheckpointStatus string
const (
    OrderPlaced            = "order_placed"             // "Order is placed"
    SellerPreparing        = "seller_preparing"         // "The sender is preparing the parcel"
    DriverHeadingPickup    = "driver_heading_pickup"    // "Driver is on the way to pick up"
    CourierPickedUp        = "courier_picked_up"        // "Courier has picked up the parcel"
    ArrivedSortingCenter   = "arrived_sorting_center"   // "Parcel has arrived at the sorting center"
    DepartedSortingCenter  = "departed_sorting_center"  // "Parcel has departed from the sorting center"
    InTransitLastMile      = "in_transit_last_mile"     // "Parcel is in transit to the last mile branch"
    ArrivedLastMileBranch  = "arrived_last_mile_branch" // "...arrived at the last mile branch"
    HandedToDriver         = "handed_to_driver"         // "Parcel has been handed over to the driver"
    OutForDelivery         = "out_for_delivery"         // "Parcel is in the process of being delivered"
    Delivered              = "delivered"                // "Parcel has been successfully delivered"
    DeliveryFailed         = "delivery_failed"          // retry / reschedule
)
```

The three-dot progress bar maps: `Shipped` (any pre-out-for-delivery) → `Out for Delivery` → `Delivered`.

---

## 3. Database schema

```sql
CREATE TYPE shipment_status AS ENUM ('created','picked_up','in_transit','out_for_delivery','delivered','failed','returned');

CREATE TABLE shipments (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tracking_no   TEXT NOT NULL UNIQUE,        -- TH268816827517W
  sub_order_id  UUID NOT NULL,
  seller_id     UUID NOT NULL,
  driver_id     UUID,                        -- assigned last-mile driver
  courier       TEXT NOT NULL DEFAULT 'SPX', -- branded courier name
  status        shipment_status NOT NULL DEFAULT 'created',
  origin        JSONB NOT NULL,              -- seller pickup {lat,lng,name}
  destination   JSONB NOT NULL,              -- customer address {lat,lng,name}
  shipped_at    TIMESTAMPTZ,
  delivered_at  TIMESTAMPTZ,
  proof_url     TEXT,                        -- "View Proof of Delivery" photo
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_shipments_suborder ON shipments(sub_order_id);
CREATE INDEX idx_shipments_driver   ON shipments(driver_id, status);

-- the timeline (append-only, strictly ordered per tracking_no)
CREATE TABLE checkpoints (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tracking_no   TEXT NOT NULL,
  status        TEXT NOT NULL,               -- CheckpointStatus
  description   TEXT NOT NULL,               -- localized line shown to customer
  location_name TEXT,                        -- "ATHON-B - ธนบุรี"
  lat           NUMERIC(9,6),
  lng           NUMERIC(9,6),
  scanned_by    UUID,                        -- driver/hub operator
  occurred_at   TIMESTAMPTZ NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_checkpoints_tracking ON checkpoints(tracking_no, occurred_at DESC);

-- GPS breadcrumbs → route polyline (high volume, short retention)
CREATE TABLE location_points (
  id            BIGSERIAL PRIMARY KEY,
  tracking_no   TEXT NOT NULL,
  lat           NUMERIC(9,6) NOT NULL,
  lng           NUMERIC(9,6) NOT NULL,
  recorded_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_locpoints_tracking ON location_points(tracking_no, recorded_at);
-- downsampled + purged after delivery (raw GPS kept 24h)

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 4. The Kafka skill-builder (why this service is the showcase)

```
delivery.checkpoint   key = tracking_no   → STRICT ORDER per parcel guaranteed by partition key
delivery.location     key = tracking_no   → high-volume firehose (GPS every 30–60s × N drivers)
delivery.status.changed key = tracking_no → order-service + notification + cashpilotx
```

Practiced here:
- **Partition by `tracking_no`** → a parcel's timeline can never arrive out of order.
- **High-throughput producer** → tune batch + linger for the GPS firehose (see master ⚠ GAP 3).
- **Independent consumer groups** → `route-builder` and `notification` both read `delivery.checkpoint`.
- **Stream processing** → route-builder consumes `delivery.location`, downsamples (1 pt/100m), upserts the polyline.
- **DLQ** → malformed driver payloads go to `delivery.checkpoint.dlq`, never block the partition.
- **Idempotent consumer** → dedupe on `event_id` (driver app may retry on flaky mobile network).

---

## 5. Driver app → ingestion

```
Driver scans barcode at a hub / does an action:
  POST /api/v1/driver/checkpoint
    { tracking_no, status, location_name, lat, lng, occurred_at }
  → validate driver owns this shipment
  → publish delivery.checkpoint (key=tracking_no)
  → AddCheckpointTx stores row; if status terminal → delivery.status.changed

Driver GPS heartbeat (client buffers, sends every ~60s):
  POST /api/v1/driver/location
    { tracking_no, points:[{lat,lng,recorded_at}, ...] }
  → publish delivery.location (one message, batched points)
  → route-builder consumer downsamples → location_points + pushes to WS hub
```

---

## 6. Customer side — tracking + live map

```
GET  /api/v1/tracking/{trackingNo}
  → {
      status, courier, tracking_no,
      progress: ["shipped","out_for_delivery","delivered"],   // the 3-dot bar
      timeline: [ {status, description, location_name, occurred_at}, ... ],  // newest first
      route:    [ {lat,lng,recorded_at}, ... ],               // polyline for the map
      current_location: {lat,lng},                            // moving pin
      destination: {lat,lng,name}
    }

WSS  /api/v1/tracking/{trackingNo}/live
  → server pushes:
      { type:"location", lat, lng, at }          // pin moves in real time
      { type:"checkpoint", status, description }  // timeline grows live
```

WebSocket hub uses **Redis pub/sub** to fan out across multiple delivery-service instances:
a driver's GPS lands on instance A, but the customer's socket is on instance B → Redis bridges them.

---

## 7. gRPC (internal)

```
delivery.GetShipmentBySubOrder(sub_order_id) → tracking_no, status   (order-service)
delivery.AssignDriver(shipment_id, driver_id) → ok                    (dispatch/admin)
```

---

## 8. Kafka summary

| Direction | Topic | Key | Notes |
|---|---|---|---|
| Consume | `suborder.paid` | sub_order_id | create shipment + tracking_no |
| Consume | `order.cancelled` | parent_order_id | cancel shipment |
| Produce | `delivery.checkpoint` | tracking_no | ordered timeline |
| Produce | `delivery.location` | tracking_no | GPS firehose (24h retention) |
| Produce | `delivery.status.changed` | tracking_no | → order, notification, cashpilotx |

---

## 9. Transaction functions

```
CreateShipmentTx     suborder.paid → generate tracking_no (TH + check digit) → INSERT shipment
                                    → first checkpoint 'order_placed'
AddCheckpointTx      INSERT checkpoint; if terminal status → UPDATE shipment.status
                                    → outbox delivery.status.changed
RecordRouteTx        upsert downsampled location_points; broadcast to WS hub
MarkDeliveredTx      status=delivered, delivered_at, proof_url
                                    → delivery.status.changed(delivered) → order.delivered downstream
```

---

## 10. Open items

- ⚠ Resolve master **GAP 3** (GPS volume / partition count / downsample strategy).
- ⚠ Driver assignment & dispatch logic (nearest driver? zone-based?) — out of MVP scope; manual assign first.
- ⚠ Map provider for the frontend polyline (Mapbox vs Google) — also affects geocoding in user-service.
- ⚠ Proof-of-delivery photo upload goes through file-service (signed URL) — wire in Sprint 3.
- COD: on `MarkDeliveredTx` with COD order, emit a `cod.collected` event → payment-service emits `order.paid`.
