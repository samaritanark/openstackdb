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
-- Manila 2025.2 Share.instance sorts by status, then prefers active replicas
-- unless the first sorted status is replication_change. Reverting precedes
-- replication_change, so its presence restores active-replica preference.
-- Upstream leaves equal/unknown-status ties to relationship order; use instance
-- ID for a stable tie break. All instance metadata comes from the same winner.
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
LEFT JOIN share_instances si ON si.id = (
    SELECT candidate.id
    FROM share_instances candidate
    WHERE candidate.share_id = s.id AND candidate.deleted = 'False'
    ORDER BY
        CASE
            WHEN EXISTS (
                SELECT 1 FROM share_instances reverting
                WHERE reverting.share_id = candidate.share_id
                  AND reverting.deleted = 'False' AND reverting.status = 'reverting'
            ) OR NOT EXISTS (
                SELECT 1 FROM share_instances promoting
                WHERE promoting.share_id = candidate.share_id
                  AND promoting.deleted = 'False' AND promoting.status = 'replication_change'
            ) THEN CASE WHEN candidate.replica_state = 'active' THEN 0 ELSE 1 END
            ELSE 0
        END,
        CASE candidate.status
            WHEN 'reverting' THEN 0
            WHEN 'replication_change' THEN 1
            WHEN 'migrating' THEN 2
            WHEN 'server_migrating' THEN 3
            WHEN 'available' THEN 4
            WHEN 'error' THEN 5
            -- Unlisted, nontransitional statuses precede transitional statuses.
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
            ELSE 6
        END,
        candidate.id
    LIMIT 1
)
LEFT JOIN share_types st ON si.share_type_id = st.id AND st.deleted = 'False'
LEFT JOIN availability_zones az ON si.availability_zone_id = az.id AND az.deleted = 'False'
WHERE s.deleted = 'False'
ORDER BY s.created_at, s.id;

-- name: QuotaGetProjectLimits :many
-- Quota tables use integer soft deletion: active = 0, soft-deleted = id.
SELECT
    project_id,
    resource,
    hard_limit
FROM
    quotas
WHERE
    deleted = 0 AND project_id <> '';

-- name: QuotaUsageGetAll :many
-- Raw accounting rows across users and share types; do not aggregate.
SELECT
    project_id,
    resource,
    in_use,
    reserved
FROM
    quota_usages
WHERE
    deleted = 0 AND project_id <> '';

-- name: QuotaGetDefaultLimits :many
-- The persisted 'default' class overrides service configuration defaults.
SELECT resource, hard_limit
FROM quota_classes
WHERE deleted = 0 AND class_name = 'default';

-- name: QuotaGetUserLimits :many
-- User overrides are independent of project and share-type overrides.
SELECT project_id, user_id, resource, hard_limit
FROM project_user_quotas
WHERE deleted = 0 AND project_id <> '';

-- name: QuotaGetShareTypeLimits :many
SELECT q.project_id, q.share_type_id, COALESCE(st.name, '') AS share_type_name,
       q.resource, q.hard_limit
FROM project_share_type_quotas q
JOIN share_types st ON st.id = q.share_type_id AND st.deleted = 'False'
WHERE q.deleted = 0 AND q.project_id <> '';

-- name: QuotaUsageGetProjectTotals :many
-- Manila _quota_usage_get_all(project): sum users only where share_type_id
-- IS NULL. Share-type accounting duplicates the same allocations and must
-- never be added to project totals. A NULL user is a legacy/project row.
SELECT project_id, resource,
       CAST(SUM(COALESCE(in_use, 0)) AS SIGNED) AS in_use,
       CAST(SUM(COALESCE(reserved, 0)) AS SIGNED) AS reserved
FROM quota_usages
WHERE deleted = 0 AND project_id <> '' AND share_type_id IS NULL
GROUP BY project_id, resource;

-- name: QuotaUsageGetShareTypeTotals :many
-- Keep share-type usage separate from the per-user project accounting.
SELECT u.project_id, u.share_type_id, COALESCE(st.name, '') AS share_type_name,
       u.resource,
       CAST(SUM(COALESCE(u.in_use, 0)) AS SIGNED) AS in_use,
       CAST(SUM(COALESCE(u.reserved, 0)) AS SIGNED) AS reserved
FROM quota_usages u
JOIN share_types st ON st.id = u.share_type_id AND st.deleted = 'False'
WHERE u.deleted = 0 AND u.project_id <> ''
GROUP BY u.project_id, u.share_type_id, st.name, u.resource;
