//go:build integration

package tests

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	cinder "github.com/vexxhost/openstackdb/cinder/db"
	"github.com/vexxhost/openstackdb/internal/testutil"
)

// The fixture includes soft-deleted rows, NULL and empty project IDs, a
// duplicate active limit, NULL limits, and usage for a deleted volume type.
func TestCinderQuotas(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewMySQLContainer(t, "cinder-quotas", "../sql/cinder/schema.sql", "../sql/cinder/indexes.sql")
	fixture, err := os.ReadFile("fixtures/cinder_quotas.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, db, string(fixture))
	q := cinder.New(db)
	s := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	n := func(v int32) sql.NullInt32 { return sql.NullInt32{Int32: v, Valid: true} }
	limit := func(project, resource string, hard sql.NullInt32) cinder.QuotaGetAllByProjectRow {
		return cinder.QuotaGetAllByProjectRow{ProjectID: s(project), Resource: resource, HardLimit: hard}
	}
	use := func(project, resource string, inUse, reserved int32) cinder.QuotaUsageGetAllByProjectRow {
		return cinder.QuotaUsageGetAllByProjectRow{ProjectID: s(project), Resource: s(resource), InUse: inUse, Reserved: reserved}
	}
	def := func(resource string, hard sql.NullInt32) cinder.QuotaClassGetDefaultsRow {
		return cinder.QuotaClassGetDefaultsRow{Resource: s(resource), HardLimit: hard}
	}

	// Raw rows in project, resource, id order. Duplicates are kept.
	limits, err := q.QuotaGetAllByProject(ctx)
	require.NoError(t, err)
	require.Equal(t, []cinder.QuotaGetAllByProjectRow{
		limit("bad-null-limit", "volumes", sql.NullInt32{}),
		limit("project-a", "gigabytes_fast_ssd", n(300)),
		limit("project-a", "snapshots", n(-1)),
		limit("project-a", "volumes", n(0)),
		limit("project-a", "volumes", n(9)),
	}, limits)

	usage, err := q.QuotaUsageGetAllByProject(ctx)
	require.NoError(t, err)
	require.Equal(t, []cinder.QuotaUsageGetAllByProjectRow{
		use("project-a", "gigabytes", 100, 15),
		use("project-a", "gigabytes_fast_ssd", 80, 10),
		use("project-a", "volumes", 2, 3),
		use("project-a", "volumes_retired", 1, 0),
		use("usage-only", "backups", 7, 4),
	}, usage)

	defaults, err := q.QuotaClassGetDefaults(ctx)
	require.NoError(t, err)
	require.Equal(t, []cinder.QuotaClassGetDefaultsRow{
		def("backups", sql.NullInt32{}),
		def("snapshots_fast_ssd", n(8)),
		def("volumes", n(20)),
	}, defaults)

	// Keys for deleted types parse, but match no current volume type.
	types, err := q.VolumeTypeGetAll(ctx)
	require.NoError(t, err)
	current := map[string]bool{}
	for _, vt := range types {
		current[vt.Name.String] = true
	}
	matched := map[string]bool{}
	for _, row := range usage {
		if base, volumeType, ok := cinder.ParseQuotaResource(row.Resource.String); ok && volumeType != "" {
			matched[base+"/"+volumeType] = current[volumeType]
		}
	}
	require.Equal(t, map[string]bool{"gigabytes/fast_ssd": true, "volumes/retired": false}, matched)
}
