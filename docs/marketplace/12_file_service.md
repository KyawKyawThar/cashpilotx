# 12 · File Service

> Uploads, image processing, and CDN-signed URLs. Product photos, KYC docs, review images,
> proof-of-delivery shots all flow through here.
> Stack: Go · Chi · S3/MinIO · RabbitMQ (processing jobs) · PostgreSQL (metadata)

---

## 1. Responsibility

- Accept uploads (multipart or pre-signed direct-to-S3).
- Store originals in S3/MinIO; generate resized/web-optimized variants asynchronously.
- Return CDN URLs (signed for private docs like KYC; public for product photos).
- Track file ownership + purpose so other services store only the URL.

---

## 2. Why async processing via RabbitMQ

Image resize/transcode is CPU work that shouldn't block the upload request:

```
Upload → store original in S3 → enqueue RabbitMQ image.process job → return 202 + file_id
                                          │
                                   image worker: generate thumb/medium/large (libvips)
                                          │  retry → DLQ on failure
                                   update file_metadata.variants → mark ready
```

---

## 3. Database schema

```sql
CREATE TYPE file_purpose AS ENUM ('product_photo','kyc_doc','review_image','delivery_proof','avatar');
CREATE TYPE file_status  AS ENUM ('uploading','processing','ready','failed');

CREATE TABLE files (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id    UUID NOT NULL,              -- uploader (user/seller)
  purpose     file_purpose NOT NULL,
  bucket      TEXT NOT NULL,
  key         TEXT NOT NULL,              -- s3 object key
  mime        TEXT NOT NULL,
  size_bytes  BIGINT NOT NULL,
  is_private  BOOLEAN NOT NULL DEFAULT false,  -- KYC docs = private (signed URLs only)
  status      file_status NOT NULL DEFAULT 'uploading',
  variants    JSONB NOT NULL DEFAULT '{}',     -- {thumb:url, medium:url, large:url}
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_files_owner ON files(owner_id, purpose);
```

---

## 4. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/files/presign` | any | Get pre-signed PUT URL for direct-to-S3 upload |
| POST | `/api/v1/files` | any | Multipart upload (small files) → 202 + file_id |
| GET | `/api/v1/files/{id}` | owner/public | Metadata + URLs (signed if private) |
| DELETE | `/api/v1/files/{id}` | owner | Remove |

**Pre-signed flow (preferred for photos):**
```
1. client POST /files/presign {purpose, mime} → {upload_url, file_id}
2. client PUTs bytes straight to S3/MinIO (bypasses our server)
3. S3 event / client callback → enqueue image.process → variants ready
```

---

## 5. Integration points

| Service | Uses file-service for |
|---|---|
| product-service | product photos (public) |
| seller-service | KYC documents (private, signed URLs) |
| review-service | review images (public) |
| delivery-service | proof-of-delivery photos (private) |
| user-service | avatars |

No Kafka — file-service is request/response + RabbitMQ jobs. Other services just store the returned URL.

---

## 6. Open items

- ⚠ CDN in front of S3/MinIO (Cloudflare) for public assets at edge.
- ⚠ Virus/content scanning on KYC + review uploads before marking ready.
- ⚠ Signed-URL TTL for private docs (KYC review window).
