-- Cinder quota accounting. Type names are encoded whole in resource keys.
-- 'retired' is a deleted type whose keys remain. NULL and empty project IDs
-- are excluded by the quota queries.
INSERT INTO volume_types (id, name, deleted) VALUES
 ('type-1', 'fast_ssd', 0), ('type-2', '__DEFAULT__', 0),
 ('type-gone', 'retired', 1);
INSERT INTO quotas (id, project_id, resource, hard_limit, deleted) VALUES
 (1, 'project-a', 'volumes', 0, 0), (2, 'project-a', 'snapshots', -1, 0),
 (3, 'project-a', 'gigabytes_fast_ssd', 300, 0),
 (4, 'project-a', 'volumes', 999, 4), (5, 'bad-null-limit', 'volumes', NULL, 0),
 (6, NULL, 'volumes', 5, 0), (7, '', 'volumes', 6, 0),
 (8, 'project-a', 'volumes', 9, 0);
INSERT INTO quota_usages (id, project_id, resource, in_use, reserved, deleted) VALUES
 (1, 'project-a', 'volumes', 2, 3, 0), (2, 'project-a', 'gigabytes', 100, 15, 0),
 (3, 'project-a', 'gigabytes_fast_ssd', 80, 10, 0),
 (4, 'usage-only', 'backups', 7, 4, 0), (5, 'project-a', 'volumes', 999, 99, 5),
 (6, 'project-a', 'volumes_retired', 1, 0, 0),
 (7, NULL, 'volumes', 1, 1, 0), (8, '', 'volumes', 1, 1, 0);
INSERT INTO quota_classes (id, class_name, resource, hard_limit, deleted) VALUES
 (1, 'default', 'volumes', 20, 0), (2, 'default', 'snapshots_fast_ssd', 8, 0),
 (3, 'default', 'volumes', 999, 3), (4, 'custom', 'volumes', 1, 0),
 (5, 'default', 'backups', NULL, 0);
