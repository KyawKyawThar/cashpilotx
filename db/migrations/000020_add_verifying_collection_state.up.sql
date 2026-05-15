-- Add 'verifying' between promised and paid:
-- customer says "I've already transferred" → owner checks bank before recording payment.
ALTER TABLE invoices
    DROP CONSTRAINT invoice_collection_state_check,
    ADD CONSTRAINT invoice_collection_state_check CHECK (
        collection_state IN (
            'pending','reminded','no_response',
            'negotiating','promised','verifying','paid','defaulted'
        )
    );
