-- Move any 'verifying' invoices back to 'promised' before dropping the value.
UPDATE invoices SET collection_state = 'promised' WHERE collection_state = 'verifying';

ALTER TABLE invoices
    DROP CONSTRAINT invoice_collection_state_check,
    ADD CONSTRAINT invoice_collection_state_check CHECK (
        collection_state IN (
            'pending','reminded','no_response',
            'negotiating','promised','paid','defaulted'
        )
    );
