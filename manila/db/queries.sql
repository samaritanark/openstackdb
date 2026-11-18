-- name: ShareGetAllWithInstances :many
-- Multi-instance view: returns one row per nondeleted instance (or a NULL
-- instance for shares with none). Use ShareGetInventory for logical shares.
SELECT 
    s.id,
    s.display_name as name,
    s.project_id,
    s.size,
    s.share_proto,
    si.status,
    COALESCE(si.share_type_id, '') as share_type,
    COALESCE(st.name, '') as share_type_name,
    COALESCE(az.name, '') as availability_zone
FROM shares s
LEFT JOIN share_instances si ON s.id = si.share_id AND si.deleted = 'False'
LEFT JOIN share_types st ON si.share_type_id = st.id AND st.deleted = 'False'  
LEFT JOIN availability_zones az ON si.availability_zone_id = az.id AND az.deleted = 'False'
WHERE s.deleted = 'False'
ORDER BY s.created_at;

-- name: ShareGetInventory :many
-- One row per nondeleted logical share, including shares with no instances.
-- Mirrors Manila Share.instance (manila/db/sqlalchemy/models.py): sort by
-- status, then prefer active replicas unless the first sorted status is
-- replication_change. Reverting sorts before replication_change, so its
-- presence restores active-replica preference. Upstream leaves equal or
-- unknown-status ties to relationship order; instance ID is the stable tie
-- break. All instance metadata comes from the same winner.
-- The transitional ranks copy constants.TRANSITIONAL_STATUSES in order
-- (manila/common/constants.py, checked against master 2026-11-18). Re-check both
-- files when moving to a new Manila release: a missing transitional status
-- falls into the unlisted bucket and would be preferred over transitional ones.
-- Retain the existing inventory deletion scope, including retained shares with
-- is_soft_deleted set; that flag is distinct from the row's deleted marker.
SELECT
    s.id,
    s.display_name as name,
    s.project_id,
    s.size,
    s.share_proto,
    si.status,
    COALESCE(si.share_type_id, '') as share_type,
    COALESCE(st.name, '') as share_type_name,
    COALESCE(az.name, '') as availability_zone
FROM shares s
LEFT JOIN (
    SELECT
        flagged.share_id,
        flagged.status,
        flagged.share_type_id,
        flagged.availability_zone_id,
        ROW_NUMBER() OVER (
            PARTITION BY flagged.share_id
            ORDER BY
                CASE
                    WHEN flagged.has_reverting = 1 OR flagged.has_replication_change = 0
                    THEN CASE WHEN flagged.replica_state = 'active' THEN 0 ELSE 1 END
                    ELSE 0
                END,
                CASE flagged.status
                    WHEN 'reverting' THEN 0
                    WHEN 'replication_change' THEN 1
                    WHEN 'migrating' THEN 2
                    WHEN 'server_migrating' THEN 3
                    WHEN 'available' THEN 4
                    WHEN 'error' THEN 5
                    -- Unlisted, nontransitional statuses (6) precede transitional ones.
                    WHEN 'creating' THEN 7
                    WHEN 'deleting' THEN 8
                    WHEN 'manage_starting' THEN 9
                    WHEN 'unmanage_starting' THEN 10
                    WHEN 'extending' THEN 11
                    WHEN 'shrinking' THEN 12
                    WHEN 'migrating_to' THEN 14
                    WHEN 'restoring' THEN 15
                    WHEN 'server_migrating_to' THEN 18
                    WHEN 'backup_restoring' THEN 19
                    WHEN 'backup_creating' THEN 20
                    WHEN 'ensuring' THEN 21
                    WHEN 'reverting_to_snapshot' THEN 22
                    ELSE 6
                END,
                flagged.id
        ) AS preference
    FROM (
        SELECT
            candidate.id,
            candidate.share_id,
            candidate.status,
            candidate.replica_state,
            candidate.share_type_id,
            candidate.availability_zone_id,
            MAX(CASE WHEN candidate.status = 'reverting' THEN 1 ELSE 0 END)
                OVER (PARTITION BY candidate.share_id) AS has_reverting,
            MAX(CASE WHEN candidate.status = 'replication_change' THEN 1 ELSE 0 END)
                OVER (PARTITION BY candidate.share_id) AS has_replication_change
        FROM share_instances candidate
        WHERE candidate.deleted = 'False'
    ) flagged
) si ON si.share_id = s.id AND si.preference = 1
LEFT JOIN share_types st ON si.share_type_id = st.id AND st.deleted = 'False'
LEFT JOIN availability_zones az ON si.availability_zone_id = az.id AND az.deleted = 'False'
WHERE s.deleted = 'False'
ORDER BY s.created_at, s.id;

-- name: QuotaGetAllByProject :many
-- Upstream quota_get_all_by_project for every project. Quota tables use
-- integer soft deletion: active = 0, soft-deleted = the row's id.
SELECT project_id, resource, hard_limit
FROM quotas
WHERE deleted = 0 AND project_id <> ''
ORDER BY project_id, resource, id;

-- name: QuotaGetAllByProjectAndUser :many
-- Upstream quota_get_all_by_project_and_user for every project and user.
-- User overrides are independent of project and share-type overrides.
SELECT project_id, user_id, resource, hard_limit
FROM project_user_quotas
WHERE deleted = 0 AND project_id <> ''
ORDER BY project_id, user_id, resource, id;

-- name: QuotaGetAllByProjectAndShareType :many
-- Upstream quota_get_all_by_project_and_share_type for every project and type.
-- share_type_name is NULL when the share type is deleted or missing; the
-- override row is still returned.
SELECT q.project_id, q.share_type_id, st.name AS share_type_name,
       q.resource, q.hard_limit
FROM project_share_type_quotas q
LEFT JOIN share_types st ON st.id = q.share_type_id AND st.deleted = 'False'
WHERE q.deleted = 0 AND q.project_id <> ''
ORDER BY q.project_id, q.share_type_id, q.resource, q.id;

-- name: QuotaClassGetDefault :many
-- Upstream quota_class_get_default. The persisted 'default' class overrides
-- service configuration defaults.
SELECT resource, hard_limit
FROM quota_classes
WHERE deleted = 0 AND class_name = 'default'
ORDER BY resource, id;

-- name: QuotaUsageGetAllByProject :many
-- Upstream quota_usage_get_all_by_project for every project: raw per-user rows
-- with share_type_id IS NULL. Share-type accounting describes the same
-- allocations and is excluded; it is read by QuotaUsageGetAllByProjectAndShareType.
-- A NULL user_id is a legacy project-level row. in_use and reserved are
-- returned as stored, including NULL.
SELECT project_id, user_id, resource, in_use, reserved
FROM quota_usages
WHERE deleted = 0 AND project_id <> '' AND share_type_id IS NULL
ORDER BY project_id, resource, user_id, id;

-- name: QuotaUsageGetAllByProjectAndShareType :many
-- Upstream quota_usage_get_all_by_project_and_share_type for every project and
-- type: raw rows with a share_type_id. share_type_name is NULL when the share
-- type is deleted or missing; the accounting row is still returned.
SELECT u.project_id, u.share_type_id, st.name AS share_type_name, u.user_id,
       u.resource, u.in_use, u.reserved
FROM quota_usages u
LEFT JOIN share_types st ON st.id = u.share_type_id AND st.deleted = 'False'
WHERE u.deleted = 0 AND u.project_id <> '' AND u.share_type_id IS NOT NULL
ORDER BY u.project_id, u.share_type_id, u.resource, u.user_id, u.id;
