-- name: PoolGetAll :many
SELECT
    id,
    project_id,
    name,
    protocol,
    lb_algorithm,
    operating_status,
    load_balancer_id,
    provisioning_status
FROM
    pool
WHERE
    provisioning_status != 'DELETED';

-- name: LoadBalancerGetAllWithVIP :many
SELECT
    lb.id,
    lb.project_id,
    lb.name,
    lb.provisioning_status,
    lb.operating_status,
    lb.provider,
    v.ip_address as vip_address
FROM
    load_balancer lb
    LEFT JOIN vip v ON lb.id = v.load_balancer_id
WHERE
    lb.provisioning_status != 'DELETED';

-- name: AmphoraGetAll :many
SELECT
    id,
    compute_id,
    status,
    load_balancer_id,
    lb_network_ip,
    ha_ip,
    role,
    cert_expiration
FROM
    amphora
WHERE
    status != 'DELETED';

-- name: QuotaGetAll :many
-- Per-project Octavia quota limits and the cached in_use_* counters that
-- Octavia's check_quota_met and decrement_quota enforce against. NULL limit
-- means no explicit override; a NULL or 0 counter is recounted by Octavia on
-- the next quota check. Octavia rejects empty project IDs (MissingProjectID),
-- so empty-string rows are excluded, matching QuotaUsageGetAll.
SELECT
    project_id,
    health_monitor,
    listener,
    load_balancer,
    member,
    pool,
    l7policy,
    l7rule,
    in_use_health_monitor,
    in_use_listener,
    in_use_load_balancer,
    in_use_member,
    in_use_pool,
    in_use_l7policy,
    in_use_l7rule
FROM
    quotas
WHERE
    project_id != ''
ORDER BY
    project_id;

-- name: QuotaUsageGetAll :many
-- Rebuild counts using Octavia check_quota_met's project/status predicate.
-- Each table is counted independently: no parent join can multiply rows.
-- Upstream defines no index on project_id or provisioning_status, so this
-- scans all seven tables; compare against QuotaGetAll's in_use_* counters to
-- detect drift rather than calling it on every scrape.
SELECT project_id, 'load_balancer' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM load_balancer
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'listener' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM listener
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'pool' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM pool
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'member' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM member
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'health_monitor' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM health_monitor
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'l7policy' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM l7policy
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id
UNION ALL
SELECT project_id, 'l7rule' AS resource, CAST(COUNT(*) AS SIGNED) AS in_use
FROM l7rule
WHERE provisioning_status != 'DELETED' AND project_id IS NOT NULL AND project_id != ''
GROUP BY project_id;
