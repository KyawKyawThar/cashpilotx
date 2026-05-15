-- Add void support to payments
ALTER TABLE payments ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ;

-- Add payment_voided event type
ALTER TYPE customer_event_type ADD VALUE IF NOT EXISTS 'payment_voided' AFTER 'payment_received';
