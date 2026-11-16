-- name: ServiceGetAll :many
SELECT
    uuid,
    host,
    `binary` as service,
    CASE
        WHEN disabled = 1 THEN 'disabled'
        ELSE 'enabled'
    END as admin_state,
    availability_zone as zone,
    disabled_reason,
    CASE
        WHEN TIMESTAMPDIFF (SECOND, updated_at, NOW()) <= 60 THEN 1
        ELSE 0
    END as state
FROM
    services
WHERE
    deleted = 0;

-- name: QuotaGetProjectLimits :many
SELECT
    q.project_id,
    q.resource,
    q.hard_limit
FROM
    quotas q
WHERE
    q.deleted = 0
    AND (q.resource IN ('gigabytes', 'backup_gigabytes') OR q.resource LIKE 'gigabytes\_%');

-- name: QuotaUsageGetAll :many
SELECT
    qu.project_id,
    qu.resource,
    qu.in_use
FROM
    quota_usages qu
WHERE
    qu.deleted = 0
    AND qu.resource IN ('gigabytes', 'backup_gigabytes');

-- name: QuotaGetAllWithUsage :many
SELECT
    q.project_id,
    q.resource,
    q.hard_limit,
    COALESCE(qu.in_use, 0) as in_use
FROM
    quotas q
    LEFT JOIN quota_usages qu ON q.project_id = qu.project_id
        AND q.resource = qu.resource
        AND qu.deleted = 0
WHERE
    q.deleted = 0;

-- name: VolumeTypeGetAllCurrent :many
SELECT
    id,
    name
FROM
    volume_types
WHERE
    deleted = 0;

-- name: SnapshotCount :one
SELECT
    COUNT(*) as count
FROM
    snapshots
WHERE
    deleted = 0;

-- name: VolumeGetAllWithAttachments :many
SELECT
    v.id,
    v.display_name as name,
    v.size,
    v.status,
    v.availability_zone,
    v.bootable,
    v.project_id,
    v.user_id,
    vt.name as volume_type,
    va.instance_uuid as server_id
FROM
    volumes v
    LEFT JOIN volume_types vt ON v.volume_type_id = vt.id
    LEFT JOIN volume_attachment va ON v.id = va.volume_id AND va.deleted = 0
WHERE
    v.deleted = 0;

-- name: VolumeGetAll :many
SELECT id, project_id, size, volume_type_id, status, created_at, deleted_at
FROM volumes WHERE deleted = 0;

-- name: QuotaGetActiveLimits :many
-- Aggregate and type-specific resources share this table. Do not join usage:
-- projects using defaults may have accounting without any override.
SELECT project_id, resource, hard_limit
FROM quotas WHERE deleted = 0;

-- name: QuotaGetActiveUsage :many
-- Accounting is already per project/resource. Never add typed resources to
-- aggregate resources, or sum duplicate active rows as if they were users.
SELECT project_id, resource, in_use, reserved
FROM quota_usages WHERE deleted = 0;

-- name: QuotaGetDefaultClassLimits :many
SELECT resource, hard_limit
FROM quota_classes WHERE deleted = 0 AND class_name = 'default';
