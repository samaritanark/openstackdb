# openstackdb

Typed, read-only Go queries for OpenStack service databases. Extracted from
[VEXXHOST's OpenStack Database Exporter](https://github.com/vexxhost/openstack_database_exporter).
The module has no Prometheus or OpenStack API client dependency in its runtime packages.

## Layout

Packages follow service boundaries and upstream DB API naming where an operation
corresponds. Nova keeps its main (cell) and API databases separate. Octavia exposes
repository objects. Placement queries live under `placement/objects`.

| Package | Examples |
| --- | --- |
| `nova/db/main` | `InstanceGetAllByFilters`, `InstanceGetByUUID`, `ServiceGetAll`, `ComputeNodeGetAll` |
| `nova/db/api` | `FlavorGetAll`, `AggregateGetAll`, quota projections |
| `cinder/db` | `ServiceGetAll`, `QuotaGetProjectLimits`, `QuotaUsageGetAll` |
| `glance/db` | `ImageGetAll` |
| `keystone/db` | `ListProjects`, `ListDomains`, `ListUsers` |
| `heat/db` | `StackGetAll` |
| `ironic/db` | `GetNodeList` |
| `magnum/db` | `GetClusterList` |
| `manila/db` | `ShareGetAllWithInstances`, `ShareGetInventory`, `QuotaGetAllByProject`, `QuotaUsageGetAllByProject` |
| `neutron/db` | `GetAgents`, `GetHARouterAgentPortBindingsWithAgents`, `GetRouters` |
| `octavia/db/repositories` | `PoolRepository`, `LoadBalancerRepository`, `AmphoraRepository` |
| `placement/objects` | Resource provider inventory and allocation projections |

## Example

```go
import (
    "context"
    "time"

    "github.com/vexxhost/openstackdb"
    nova "github.com/vexxhost/openstackdb/nova/db/main"
)

func instanceCount(ctx context.Context, databaseURL, projectID string) (int, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    conn, err := openstackdb.ConnectContext(ctx, databaseURL, openstackdb.ConnectionOptions{})
    if err != nil { return 0, err }
    defer conn.Close()

    instances, err := nova.New(conn).InstanceGetAllByFilters(ctx, nova.InstanceFilters{
        ProjectIDs: []string{projectID},
        VMStates: []string{"active", "stopped"},
    })
    return len(instances), err
}
```

Existing `*sql.DB` and `*sql.Tx` values can also be passed to service constructors.
Generated query sets support `WithTx`. Callers own pools, deadlines, region/cell
selection, credentials, and authorization. Use database accounts with SELECT access.
The library does not discover cells or authorize requests through Keystone.

## Filter semantics

Nova instances, Cinder volumes, and Glance images have typed `...GetAllByFilters`
methods. Fields combine with AND; values in a slice combine with OR. A nil slice
means unrestricted, while a non-nil empty slice matches nothing. Filter values
are SQL parameters, never SQL fragments. Comparisons use the database collation.

Soft-deleted records are excluded by default. Set `Deleted` to
`openstackdb.IncludeDeleted` or `openstackdb.OnlyDeleted` explicitly.
`DeletedAfter` and `CreatedBefore` must either both be zero or define a nonempty
half-open lifetime window: `created_at < end AND (deleted_at IS NULL OR deleted_at > start)`.
Times are converted to UTC; these predicates assume UTC database timestamps.
Nova `InstanceFilters.Archive` selects primary tables, shadow tables, or both.
`WithExtra` joins the corresponding instance-extra table for raw flavor JSON.
Missing extra rows preserve the instance with a null flavor. Missing requested
archive tables are errors; the library never substitutes an incomplete result.
Use `WithTx` with an appropriate transaction isolation level for a consistent
primary/shadow snapshot. Rows carry an `Archived` flag; no deduplication is applied.

`IncludeStartBoundary` includes rows deleted exactly at the window start.
`DeletedAtIsNull` filters by deletion timestamp independently of the soft-delete
flag; combine it with `Deleted: openstackdb.IncludeDeleted` for timestamp-only
selection. Default interval/deletion semantics remain unchanged.

Filtered Nova results use `InstanceRecord` (including `CreatedAt`, `DeletedAt`,
`Hostname`, and optional `Flavor`). Filtered Glance results use `ImageRecord`
(including `DeletedAt`). These embed the original exporter row projections.
Cinder records preserve nullable volume type IDs for older service schemas.

## Metadata and lookup operations

- Nova `InstanceExtraGetByInstanceUUID` reads flavor JSON, with optional archived
  access, following upstream `instance_extra_get_by_instance_uuid` naming.
- Nova `InstanceTypeGetAll` reads legacy type identities, following historical
  `instance_type_get_all` naming. `Inactive` includes deleted types; `Archived`
  selects `shadow_instance_types`. Modern `FlavorGetAll` remains in `nova/db/api`.
- Cinder `VolumeTypeGetAll(ctx, VolumeTypeOptions{Inactive: true})` includes deleted
  types, matching the upstream `inactive` option. Existing calls remain valid.
- Keystone `GetProject` returns a project by ID, including disabled projects.

Record sizes and flavor JSON are returned without rounding, unit conversion, or
interpretation. Callers own region aggregation and configured default-type fallback.

The richer filtered Nova/Glance result types change explicit result declarations
from the initial release; field access remains promoted through the embedded row.
The fixed exporter queries and their result projections remain unchanged.

Single-resource methods return `sql.ErrNoRows` for absent or excluded records.
List methods do not promise ordering or pagination; constrain project/resource
filters and use deadlines for large deployments.

## Upstream correspondence and compatibility

This is a read-only subset inspired by upstream APIs, not a drop-in implementation
of their Python APIs. Results are typed projections, not complete ORM objects.
See [API mapping](API.md) for naming and retained query semantics.

Exporter-oriented joins and aggregates remain explicit extensions. In particular,
Cinder `VolumeGetAllWithAttachments` can return multiple rows per volume;
`VolumeGetAll` and `VolumeGetAllByFilters` return one row per volume. Manila's
`ShareGetAllWithInstances` and Octavia's `GetAllWithVIP` similarly describe joins.
`ShareGetInventory` returns exactly one row per nondeleted logical share,
including shares without instances. It follows Manila's `Share.instance`:
status precedence, active-replica preference, and the replication-change
exception. Instance ID resolves otherwise unspecified ties (including writable
replication with multiple active replicas). Status, AZ and type all come from
the same representative instance, chosen in one pass with window functions.
The status ranking copies Manila's `constants.TRANSITIONAL_STATUSES`; re-check
it against `manila/common/constants.py` and `Share.instance` in
`manila/db/sqlalchemy/models.py` when moving to a new Manila release, because a
missing transitional status would be preferred over the listed ones. Retained
`is_soft_deleted` shares remain in inventory, preserving the existing deletion
scope; row-soft-deleted shares and instances are excluded. This is inventory,
not quota accounting.

No resource mutation, schema migration, or OpenStack request-context behavior is
implemented. Query compatibility depends on the deployed service schema.

`schema.Files` embeds the minimal schema/index fixtures used by integration tests.
They are test inputs, not production migrations or a claim of support for every
OpenStack release. Some inherited queries require indexes present in the fixtures.

## Development

Requires Go 1.25.5 or later. SQL sources live alongside their generated Go packages.
Do not edit `queries.sql.go`, `models.go`, or generated `db.go` files by hand.

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
sqlc generate
go test -race ./...
go vet ./...
# Requires Docker; runs every query against MariaDB and seeded filter scenarios.
go test -race -tags integration -timeout 15m ./...
```

Exporter collector tests continue to verify metric output in the exporter
repository. This module owns connection, predicate, and database query tests.

## Manila quota accounting

The Manila quota queries are named after Manila's DB API functions. Each one
reads every project at once rather than one project, and returns raw rows
rather than a per-project dictionary:

| Go method | Manila function | Table |
| --- | --- | --- |
| `QuotaGetAllByProject` | `quota_get_all_by_project` | `quotas` |
| `QuotaGetAllByProjectAndUser` | `quota_get_all_by_project_and_user` | `project_user_quotas` |
| `QuotaGetAllByProjectAndShareType` | `quota_get_all_by_project_and_share_type` | `project_share_type_quotas` |
| `QuotaClassGetDefault` | `quota_class_get_default` | `quota_classes` |
| `QuotaUsageGetAllByProject` | `quota_usage_get_all_by_project` | `quota_usages`, `share_type_id IS NULL` |
| `QuotaUsageGetAllByProjectAndShareType` | `quota_usage_get_all_by_project_and_share_type` | `quota_usages`, `share_type_id IS NOT NULL` |

Rows with a NULL or empty project ID are excluded. Results are ordered by their
identifying columns and row ID, so callers that collapse duplicates get a
stable result.

Usage rows are not aggregated. `in_use` and `reserved` are returned as stored,
including NULL, so callers can tell malformed rows from zero usage. To get
Manila's project totals, sum `QuotaUsageGetAllByProject` rows by project and
resource; this matches `_quota_usage_get_all`, which sums users (including
legacy NULL-user rows) and excludes share-type rows. Share-type accounting
describes the same allocations: read it with
`QuotaUsageGetAllByProjectAndShareType` and never add it to project totals.
`reserved` already counts open reservations; do not add `reservations` again.

The share-type queries left-join `share_types`. When a share type is deleted or
missing, its limits and usage are still returned with a NULL `ShareTypeName`, so
per-type totals reconcile with project totals.

The generated quota models use `sql.NullInt32` for integer `Deleted` fields:
active rows have `0`, and soft-deleted rows store their row ID. The fixture has
no database default for these columns; Manila supplies `0` through SQLAlchemy.
Share and instance deletion markers retain their existing string representation.

User overrides live in `project_user_quotas`, separately from project overrides
in `quotas` and type overrides in `project_share_type_quotas`. Never sum limits
or substitute user limits for project limits. A NULL hard limit is preserved
so callers can detect invalid or legacy data; Manila enforces numeric limits,
using `-1` for unlimited. The caller supplies service configuration defaults
and resolves missing limits: share-type override → project override →
persisted `default` class → service configuration. Non-default classes need
request-context information that these database tables do not provide.

These queries read Manila's quota accounting, not rebuilt object counts or
physical storage use. Stale accounting, including usage left after object
removal, remains visible until Manila reconciles it. No writes or reconciliation
are performed. The schema files are test fixtures, not production migrations.

```sh
go test -race ./manila/db
# Requires Docker and a disposable MariaDB container:
go test -race -tags integration ./manila/db
```

## License

Apache-2.0. See LICENSE and NOTICE.
