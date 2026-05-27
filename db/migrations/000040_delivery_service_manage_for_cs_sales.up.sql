-- Grant delivery_service:manage to cs_team and sales roles.
INSERT INTO role_permissions (role, permission_id, scope)
SELECT r.role, p.id, r.scope
FROM (VALUES
    ('cs_team'::user_role, 'global'),
    ('sales'::user_role,   'own')
) AS r(role, scope)
CROSS JOIN permissions p
WHERE p.resource = 'delivery_service' AND p.action = 'manage'
ON CONFLICT (role, permission_id) DO NOTHING;
