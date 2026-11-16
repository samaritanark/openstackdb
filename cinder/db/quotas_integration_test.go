//go:build integration

package cinder

import (
	"context"
	"database/sql"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
)

func TestQuotaRepositoryMariaDB(t *testing.T) {
	conn := testutil.NewMySQLContainer(t, "cinder-quotas", "../../schema/cinder/schema.sql", "../../schema/cinder/indexes.sql")
	fixture, err := os.ReadFile("../../tests/fixtures/cinder_quotas.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, conn, string(fixture))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var version string
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version))
	t.Logf("MariaDB version: %s", version)
	s := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	n := func(v int32) sql.NullInt32 { return sql.NullInt32{Int32: v, Valid: true} }

	t.Run("independent_accounting", func(t *testing.T) {
		r := NewQuotaRepository(conn)
		limits, err := r.GetProjectLimits(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetActiveLimitsRow{
			{s("project-a"), "volumes", n(0)},
			{s("project-a"), "snapshots", n(-1)},
			{s("project-a"), "gigabytes_fast_ssd", n(300)},
			{s("bad-null-limit"), "volumes", sql.NullInt32{}},
			{sql.NullString{}, "volumes", n(5)},
		}, limits)
		usage, err := r.GetProjectUsage(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetActiveUsageRow{
			{s("project-a"), s("volumes"), 2, 3},
			{s("project-a"), s("gigabytes"), 100, 15},
			{s("project-a"), s("gigabytes_fast_ssd"), 80, 10},
			{s("usage-only"), s("backups"), 7, 4},
		}, usage)
		defaults, err := r.GetDefaultLimits(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetDefaultClassLimitsRow{
			{s("volumes"), n(20)}, {s("snapshots_fast_ssd"), n(8)},
		}, defaults)
		types, err := r.GetVolumeTypes(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []VolumeTypeGetAllCurrentRow{
			{"type-1", s("fast_ssd")}, {"type-2", s("__DEFAULT__")},
		}, types)
	})

	t.Run("duplicates_nulls_and_boundaries", func(t *testing.T) {
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		for _, stmt := range []string{
			`INSERT INTO quotas(project_id,resource,hard_limit,deleted) VALUES
			 ('project-a','volumes',9,0), ('','',2147483647,0),
			 ('minimum','volumes',-2147483648,0), ('excluded','volumes',999,NULL)`,
			`INSERT INTO quota_usages(project_id,resource,in_use,reserved,deleted) VALUES
			 ('project-a','volumes',8,9,0), (NULL,NULL,-2147483648,2147483647,0),
			 ('','',0,-1,0), ('excluded','volumes',999,999,NULL)`,
			`INSERT INTO quota_classes(class_name,resource,hard_limit,deleted) VALUES
			 ('default','volumes',0,0), ('default','volumes',-1,0),
			 ('default',NULL,NULL,0), ('default','maximum',2147483647,0),
			 (NULL,'ignored',999,0), ('default','ignored',999,NULL)`,
		} {
			_, err := tx.ExecContext(ctx, stmt)
			require.NoError(t, err)
		}
		r := NewQuotaRepository(tx)
		limits, err := r.GetProjectLimits(ctx)
		require.NoError(t, err)
		require.Len(t, limits, 8)
		require.Contains(t, limits, QuotaGetActiveLimitsRow{s("project-a"), "volumes", n(0)})
		require.Contains(t, limits, QuotaGetActiveLimitsRow{s("project-a"), "volumes", n(9)})
		require.Contains(t, limits, QuotaGetActiveLimitsRow{s(""), "", n(math.MaxInt32)})
		require.Contains(t, limits, QuotaGetActiveLimitsRow{s("minimum"), "volumes", n(math.MinInt32)})
		usage, err := r.GetProjectUsage(ctx)
		require.NoError(t, err)
		require.Len(t, usage, 7)
		require.Contains(t, usage, QuotaGetActiveUsageRow{s("project-a"), s("volumes"), 2, 3})
		require.Contains(t, usage, QuotaGetActiveUsageRow{s("project-a"), s("volumes"), 8, 9})
		require.Contains(t, usage, QuotaGetActiveUsageRow{sql.NullString{}, sql.NullString{}, math.MinInt32, math.MaxInt32})
		require.Contains(t, usage, QuotaGetActiveUsageRow{s(""), s(""), 0, -1})
		defaults, err := r.GetDefaultLimits(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetDefaultClassLimitsRow{
			{s("volumes"), n(20)}, {s("snapshots_fast_ssd"), n(8)},
			{s("volumes"), n(0)}, {s("volumes"), n(-1)},
			{sql.NullString{}, sql.NullInt32{}}, {s("maximum"), n(math.MaxInt32)},
		}, defaults)
	})

	t.Run("legacy_quota_contracts", func(t *testing.T) {
		q := New(conn)
		limits, err := q.QuotaGetProjectLimits(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetProjectLimitsRow{
			{s("project-a"), "gigabytes_fast_ssd", n(300)},
		}, limits)
		usage, err := q.QuotaUsageGetAll(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaUsageGetAllRow{
			{s("project-a"), s("gigabytes"), 100},
		}, usage)
		joined, err := q.QuotaGetAllWithUsage(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []QuotaGetAllWithUsageRow{
			{s("project-a"), "volumes", n(0), 2},
			{s("project-a"), "snapshots", n(-1), 0},
			{s("project-a"), "gigabytes_fast_ssd", n(300), 80},
			{s("bad-null-limit"), "volumes", sql.NullInt32{}, 0},
			{sql.NullString{}, "volumes", n(5), 0},
		}, joined)
	})

	t.Run("volume_attachment_multiplicity", func(t *testing.T) {
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		for _, stmt := range []string{
			`INSERT INTO volumes(id,project_id,volume_type_id,service_uuid,deleted) VALUES
			 ('v1','project-a','type-1',NULL,0), ('v2','project-a','type-1','svc',0),
			 ('gone','project-a','type-1','svc',1)`,
			`INSERT INTO volume_attachment(id,volume_id,instance_uuid,deleted) VALUES
			 ('a1','v1','server-1',0), ('a2','v1','server-2',0), ('gone','v1','old',1)`,
		} {
			_, err := tx.ExecContext(ctx, stmt)
			require.NoError(t, err)
		}
		q := New(tx)
		volumes, err := q.VolumeGetAll(ctx)
		require.NoError(t, err)
		require.Len(t, volumes, 2)
		filtered, err := q.VolumeGetAllByFilters(ctx, VolumeFilters{ProjectIDs: []string{"project-a"}})
		require.NoError(t, err)
		require.ElementsMatch(t, volumes, filtered)
		filtered, err = q.VolumeGetAllByFilters(ctx, VolumeFilters{IDs: []string{}})
		require.NoError(t, err)
		require.Empty(t, filtered)
		joined, err := q.VolumeGetAllWithAttachments(ctx)
		require.NoError(t, err)
		require.Len(t, joined, 3)
		servers := map[string][]sql.NullString{}
		for _, row := range joined {
			require.Equal(t, s("fast_ssd"), row.VolumeType)
			servers[row.ID] = append(servers[row.ID], row.ServerID)
		}
		require.ElementsMatch(t, []sql.NullString{s("server-1"), s("server-2")}, servers["v1"])
		require.Equal(t, []sql.NullString{{}}, servers["v2"])
		// Active-volume queries must work with only the fixture indexes: no
		// forced service_uuid index may be required by either query plan.
		for _, query := range []string{VolumeGetAll, VolumeGetAllWithAttachments} {
			plan, err := tx.QueryContext(ctx, "EXPLAIN "+query)
			require.NoError(t, err)
			require.True(t, plan.Next())
			require.NoError(t, plan.Close())
			require.NoError(t, plan.Err())
		}
	})
}
