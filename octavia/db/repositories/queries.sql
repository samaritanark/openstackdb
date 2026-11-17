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
-- Per-project Octavia quota limits. NULL column means no explicit override.
SELECT
    project_id,
    health_monitor,
    listener,
    load_balancer,
    member,
    pool,
    l7policy,
    l7rule
FROM
    quotas;

-- name: LoadBalancerCountByProject :many
SELECT
    project_id,
    COUNT(*) as count
FROM
    load_balancer
WHERE
    provisioning_status != 'DELETED'
    AND project_id IS NOT NULL
    AND project_id != ''
GROUP BY
    project_id;

-- name: PoolCountByProject :many
SELECT
    project_id,
    COUNT(*) as count
FROM
    pool
WHERE
    provisioning_status != 'DELETED'
    AND project_id IS NOT NULL
    AND project_id != ''
GROUP BY
    project_id;

-- name: ListenerCountByProject :many
SELECT
    project_id,
    COUNT(*) as count
FROM
    listener
WHERE
    provisioning_status != 'DELETED'
    AND project_id IS NOT NULL
    AND project_id != ''
GROUP BY
    project_id;

-- name: QuotaUsageGetAll :many
-- Rebuild counts using Octavia check_quota_met's project/status predicate.
-- Each table is counted independently: no parent join can multiply rows.
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
