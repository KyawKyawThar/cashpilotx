DELETE FROM role_permissions
WHERE role IN ('cs_team', 'sales')
  AND permission_id = (SELECT id FROM permissions WHERE resource = 'delivery_service' AND action = 'manage');
