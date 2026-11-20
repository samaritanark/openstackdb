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
| `cinder/db` | `ServiceGetAll`, `QuotaGetAllByProject`, `QuotaUsageGetAllByProject`, `ParseQuotaResource` |
| `glance/db` | `ImageGetAll` |
| `keystone/db` | `ListProjects`, `ListDomains`, `ListUsers` |
| `heat/db` | `StackGetAll` |
| `ironic/db` | `GetNodeList` |
| `magnum/db` | `GetClusterList` |
| `manila/db` | `ShareGetAllWithInstances` |
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
No resource mutation, schema migration, or OpenStack request-context behavior is
implemented. Query compatibility depends on the deployed service schema.

`schema.Files` embeds the minimal schema/index fixtures used by integration tests.
They are test inputs, not production migrations or a claim of support for every
OpenStack release. Some inherited queries require indexes present in the fixtures.

## Cinder quota accounting

The Cinder quota queries are named after Cinder's DB API functions. Each one
reads every project at once rather than one project, and returns raw rows
rather than a per-project dictionary:

| Go method | Cinder function | Table |
| --- | --- | --- |
| `QuotaGetAllByProject` | `quota_get_all_by_project` | `quotas` |
| `QuotaUsageGetAllByProject` | `quota_usage_get_all_by_project` | `quota_usages` |
| `QuotaClassGetDefaults` | `quota_class_get_defaults` | `quota_classes`, `default` class |

Rows with a NULL or empty project ID are excluded: Cinder quota rows belong to
Keystone projects. Results are ordered by project, resource and row ID, so
callers that collapse duplicates get a stable result. Duplicate active rows,
NULL limits, zero limits and `-1` (unlimited) are returned as stored. The
existing exporter queries `QuotaGetProjectLimits`, `QuotaUsageGetAll` and
`QuotaGetAllWithUsage` keep their behavior.

Limits and usage are read separately, so usage also covers projects that have
no limit overrides. Usage is Cinder's cached accounting, not a live count of
volumes and snapshots. It can drift from the real resources until Cinder
refreshes it. `reserved` already counts open reservations.

Resource keys are either global (`volumes`, `gigabytes`, `snapshots`, `backups`,
`backup_gigabytes`, `per_volume_gigabytes`, `groups`) or per volume type, built
as `<part>_<volume type name>` where the part is `volumes`, `gigabytes` or
`snapshots`. Per-type and global accounting describe the same allocations, so
never add them together. Use `ParseQuotaResource` to split a key. Don't split
on the first underscore: that misreads `backup_gigabytes` and type names that
contain underscores. Keys hold the volume type's name, not its ID. Cinder
rewrites them when a type is renamed, but keeps them after a type is deleted,
so a parsed name may not match any type from `VolumeTypeGetAll`.

Callers resolve effective limits: a project override, then the `default` class,
then service configuration. The `quota_classes` schema is a test and
code-generation fixture, not a production migration.

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

## License

Apache-2.0. See LICENSE and NOTICE.
