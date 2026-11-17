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

### Octavia quota usage validation

`NewQuotaRepository(db).GetAll(ctx)` / `QuotaGetAll` returns stored project quota
limits for load balancers, listeners, pools, health monitors, members, L7 policies
and L7 rules. Each limit is a `sql.NullInt32`: NULL means no explicit override,
and zero and -1 remain distinct values. The caller resolves configured defaults.
This read preserves quota-table project keys, including empty strings.

`QuotaRepository.GetUsage` / `QuotaUsageGetAll` rebuilds project counts for all
seven quota resources independently, using `provisioning_status != 'DELETED'`.
It follows the count-rebuild predicate in
[Octavia 2025.2 `check_quota_met`](https://opendev.org/openstack/octavia/src/branch/stable/2025.2/octavia/db/repositories.py), not
its cached `in_use_*` columns. Differences from the enforcement counters can
persist until reconciled.
Counts include ERROR and pending statuses. Each object uses its own project and
status, including children of deleted parents or parents owned by another project.
There are no parent joins to multiply counts. Results contain only nonzero
project/resource groups, including projects without quota rows, with int64 usage.
NULL and empty project IDs are excluded by `QuotaUsageGetAll` and the
`LoadBalancerCountByProject`, `ListenerCountByProject`, and `PoolCountByProject`
queries. Valid-project counts use the same deleted-resource predicate in all
four queries. The schema fixtures contain only fields needed for these tests,
not a complete OpenStack migration or proposed production schema change.

Portable SQL semantics tests (Python standard library, in-memory SQLite):

```sh
python3 -m unittest discover -s tests -p octavia_quota_sql_test.py -v
```

Run these tests explicitly when the CI workflow does not include Python SQL-test discovery.
Project-a counts are distinct (4 through 10)
for load balancers, listeners, pools, members, health monitors, L7 policies and
L7 rules, respectively; both SQL test paths assert the exact project/resource map.
These exercise the generated SQL and schema/seed fixtures without a database service,
but do not establish MySQL/MariaDB dialect or deployment compatibility. Native
query/scanning validation against the disposable MariaDB test container is:

```sh
go test -race -tags integration -timeout 15m ./octavia/db/repositories -run TestQuotaUsageMariaDB -count=1 -v
```

This integration test requires Docker and never reads production credentials.
Regenerate query code with sqlc v1.30.0 (`sqlc generate`).

## License

Apache-2.0. See LICENSE and NOTICE.
