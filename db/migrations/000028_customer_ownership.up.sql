-- Sprint A: Customer account ownership for RBAC scope enforcement.

ALTER TABLE customers
    ADD COLUMN assigned_to             UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN credit_limit_approved_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_customers_assigned ON customers(assigned_to);
