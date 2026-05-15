-- Cannot remove enum values in Postgres; voided_at column can be dropped
ALTER TABLE payments DROP COLUMN IF EXISTS voided_at;
