-- Cinder 27.0.0 accounting. Type names are encoded whole in resource keys.
INSERT INTO volume_types (id, name, deleted) VALUES
 ('type-1', 'fast_ssd', 0), ('type-2', '__DEFAULT__', 0),
 ('type-gone', 'retired', 1);
INSERT INTO quotas (project_id, resource, hard_limit, deleted) VALUES
 ('project-a', 'volumes', 0, 0), ('project-a', 'snapshots', -1, 0),
 ('project-a', 'gigabytes_fast_ssd', 300, 0),
 ('project-a', 'volumes', 999, 1), ('bad-null-limit', 'volumes', NULL, 0),
 (NULL, 'volumes', 5, 0);
INSERT INTO quota_usages (project_id, resource, in_use, reserved, deleted) VALUES
 ('project-a', 'volumes', 2, 3, 0), ('project-a', 'gigabytes', 100, 15, 0),
 ('project-a', 'gigabytes_fast_ssd', 80, 10, 0),
 ('usage-only', 'backups', 7, 4, 0), ('project-a', 'volumes', 999, 99, 1);
INSERT INTO quota_classes (class_name, resource, hard_limit, deleted) VALUES
 ('default', 'volumes', 20, 0), ('default', 'snapshots_fast_ssd', 8, 0),
 ('default', 'volumes', 999, 1), ('custom', 'volumes', 1, 0);
