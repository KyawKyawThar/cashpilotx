-- Sprint A: Seed the full permissions + role_permissions matrix.
-- Idempotent via ON CONFLICT DO NOTHING on both tables.

-- ── 1. Permissions catalog ────────────────────────────────────────────────────
INSERT INTO permissions (resource, action, description) VALUES
    -- invoice
    ('invoice', 'view',         'View invoices'),
    ('invoice', 'view_history', 'View invoice history beyond current month'),
    ('invoice', 'create',       'Create invoices'),
    ('invoice', 'edit',         'Edit invoices'),
    ('invoice', 'delete',       'Cancel / delete invoices'),
    ('invoice', 'export',       'Export invoices to file'),
    -- customer
    ('customer', 'view',              'View customers'),
    ('customer', 'create',            'Create customers'),
    ('customer', 'edit',              'Edit customer details'),
    ('customer', 'delete',            'Delete / block customers'),
    ('customer', 'view_risk_profile', 'View customer risk profile'),
    ('customer', 'set_credit_limit',  'Set or approve credit limit'),
    -- payment
    ('payment', 'view',   'View payments'),
    ('payment', 'record', 'Record a payment'),
    ('payment', 'void',   'Void a payment'),
    -- reminder
    ('reminder', 'view',   'View reminders'),
    ('reminder', 'create', 'Schedule reminders'),
    ('reminder', 'cancel', 'Cancel reminders'),
    -- message
    ('message', 'view', 'View messages'),
    ('message', 'send', 'Send messages'),
    -- audit_log
    ('audit_log', 'view',   'View audit log'),
    ('audit_log', 'revert', 'Revert audit log actions'),
    -- report
    ('report', 'view',   'View reports'),
    ('report', 'export', 'Export reports'),
    -- recovery_queue
    ('recovery_queue', 'view',   'View recovery queue'),
    ('recovery_queue', 'action', 'Take action on recovery queue items'),
    -- product
    ('product', 'view_selling', 'View product selling price'),
    ('product', 'view_cost',    'View product cost price'),
    ('product', 'manage',       'Create, edit, deactivate products'),
    -- channel_config
    ('channel_config', 'view',   'View channel configuration'),
    ('channel_config', 'manage', 'Manage channel configuration'),
    -- delivery_service
    ('delivery_service', 'view',   'View delivery services'),
    ('delivery_service', 'manage', 'Create, edit, deactivate delivery services'),
    -- team
    ('team', 'view',                'View team members'),
    ('team', 'create_admin',        'Create admin accounts'),
    ('team', 'create_team_leader',  'Create team leader accounts'),
    ('team', 'create_team_member',  'Create team member accounts'),
    ('team', 'edit',                'Edit team member details'),
    ('team', 'delete',              'Deactivate team members'),
    ('team', 'reset_pin',           'Reset a team member PIN'),
    ('team', 'grant_permission',    'Grant per-user permission exceptions')
ON CONFLICT (resource, action, qualifier) DO NOTHING;


-- ── 2. super_admin — global scope on everything ───────────────────────────────
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'super_admin', id, 'global'
FROM permissions
ON CONFLICT (role, permission_id) DO NOTHING;


-- ── 3. admin — global scope, minus super_admin-only actions ──────────────────
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'admin', id, 'global'
FROM permissions
WHERE NOT (resource = 'team'         AND action IN ('create_admin', 'grant_permission'))
  AND NOT (resource = 'channel_config' AND action = 'manage')
  AND NOT (resource = 'audit_log'    AND action = 'revert')
ON CONFLICT (role, permission_id) DO NOTHING;


-- ── 4. team_leader — team scope ──────────────────────────────────────────────
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'team_leader', id, 'team'
FROM permissions
WHERE (resource = 'invoice'  AND action IN ('view', 'view_history', 'create', 'edit', 'delete'))
   OR (resource = 'customer' AND action IN ('view', 'create', 'edit', 'delete', 'view_risk_profile'))
   OR (resource = 'payment'  AND action IN ('view', 'record'))
   OR (resource = 'reminder' AND action IN ('view', 'create', 'cancel'))
   OR (resource = 'message'  AND action IN ('view', 'send'))
   OR (resource = 'audit_log' AND action = 'view')
   OR (resource = 'product'  AND action = 'view_selling')
   OR (resource = 'delivery_service' AND action = 'view')
   OR (resource = 'team'     AND action IN ('view', 'create_team_member', 'edit', 'reset_pin'))
ON CONFLICT (role, permission_id) DO NOTHING;


-- ── 5. team_member — own scope ────────────────────────────────────────────────
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'team_member', id, 'own'
FROM permissions
WHERE (resource = 'invoice'  AND action IN ('view', 'create'))
   OR (resource = 'customer' AND action IN ('view', 'create', 'edit'))
   OR (resource = 'payment'  AND action IN ('view', 'record'))
   OR (resource = 'reminder' AND action = 'view')
   OR (resource = 'message'  AND action IN ('view', 'send'))
   OR (resource = 'product'  AND action = 'view_selling')
   OR (resource = 'delivery_service' AND action = 'view')
ON CONFLICT (role, permission_id) DO NOTHING;
