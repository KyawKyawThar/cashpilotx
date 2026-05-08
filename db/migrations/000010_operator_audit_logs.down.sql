-- ═══════════════════════════════════════════════════════
-- DROP TRIGGERS
-- ═══════════════════════════════════════════════════════

DROP TRIGGER IF EXISTS trg_prevent_audit_log_mutation
ON operator_audit_log;

DROP TRIGGER IF EXISTS trg_prevent_audit_log_delete
ON operator_audit_log;



-- ═══════════════════════════════════════════════════════
-- DROP FUNCTIONS
-- ═══════════════════════════════════════════════════════

DROP FUNCTION IF EXISTS prevent_audit_log_mutation();

DROP FUNCTION IF EXISTS prevent_audit_log_delete();



-- ═══════════════════════════════════════════════════════
-- DROP INDEXES
-- ═══════════════════════════════════════════════════════

DROP INDEX IF EXISTS idx_audit_user_date;

DROP INDEX IF EXISTS idx_audit_type;

DROP INDEX IF EXISTS idx_audit_invoice;

DROP INDEX IF EXISTS idx_audit_customer;

DROP INDEX IF EXISTS idx_audit_payment;

DROP INDEX IF EXISTS idx_audit_message;

DROP INDEX IF EXISTS idx_audit_reminder;

DROP INDEX IF EXISTS idx_audit_entity;

DROP INDEX IF EXISTS idx_audit_reverted;

DROP INDEX IF EXISTS idx_audit_request_id;

DROP INDEX IF EXISTS idx_audit_metadata_gin;



-- ═══════════════════════════════════════════════════════
-- DROP TABLE
-- ═══════════════════════════════════════════════════════

DROP TABLE IF EXISTS operator_audit_log;



-- ═══════════════════════════════════════════════════════
-- DROP ENUMS
-- ═══════════════════════════════════════════════════════

DROP TYPE IF EXISTS audit_entity_type;

DROP TYPE IF EXISTS audit_action_type;