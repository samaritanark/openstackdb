//go:build integration

package manila

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
)

func TestQuotaRepositoryMariaDB(t *testing.T) {
	db := testutil.NewMySQLContainer(t, "manila", "../../sql/manila/prereqs.sql", "../../sql/manila/schema.sql")
	fixture, err := os.ReadFile("../../tests/fixtures/manila_quotas.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, db, string(fixture))
	for _, table := range strings.Fields("quotas quota_usages quota_classes project_user_quotas project_share_type_quotas") {
		var dataType string
		var noDefault bool
		err := db.QueryRow("SELECT DATA_TYPE, (COLUMN_DEFAULT IS NULL OR COLUMN_DEFAULT = 'NULL') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'deleted'", table).Scan(&dataType, &noDefault)
		require.NoError(t, err)
		require.Equal(t, "int", dataType, table)
		require.True(t, noDefault, table)
		var invalid, deleted int
		err = db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE deleted <> 0 AND deleted <> id").Scan(&invalid)
		require.NoError(t, err)
		require.Zero(t, invalid, table)
		err = db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE deleted = id").Scan(&deleted)
		require.NoError(t, err)
		require.Positive(t, deleted, table)
	}
	r := NewQuotaRepository(db)
	ctx := context.Background()
	raw, err := New(db).QuotaUsageGetAll(ctx)
	require.NoError(t, err)
	require.Len(t, raw, 38)
	duplicates := 0
	for _, row := range raw {
		require.True(t, row.ProjectID.Valid)
		require.NotEmpty(t, row.ProjectID.String)
		require.NotEqual(t, "deleted-only", row.ProjectID.String)
		if row.ProjectID.String == "usage-only" && row.Resource.String == "gigabytes" {
			require.Equal(t, sql.NullInt32{Int32: 2000000000, Valid: true}, row.InUse)
			duplicates++
		}
		if row.ProjectID.String == "null-usage" {
			require.False(t, row.InUse.Valid)
			require.False(t, row.Reserved.Valid)
		}
	}
	require.Equal(t, 2, duplicates)
	usage, err := r.GetProjectUsage(ctx)
	require.NoError(t, err)
	got := map[string][2]int64{}
	for _, row := range usage {
		key := row.ProjectID.String + "/" + row.Resource.String
		require.NotContains(t, got, key)
		got[key] = [2]int64{row.InUse, row.Reserved}
	}
	expected := map[string][2]int64{}
	for i, resource := range strings.Fields("shares gigabytes snapshots snapshot_gigabytes share_networks share_replicas replica_gigabytes share_groups share_group_snapshots backups backup_gigabytes") {
		expected["project-a/"+resource] = [2]int64{int64(i + 11), 3}
	}
	expected["project-a/shares"] = [2]int64{13, 7}
	expected["usage-only/shares"] = [2]int64{5, 2}
	expected["usage-only/gigabytes"] = [2]int64{4000000000, 0}
	expected["null-usage/shares"] = [2]int64{0, 0}
	require.Equal(t, expected, got)
	types, err := r.GetShareTypeUsage(ctx)
	require.NoError(t, err)
	typed := map[string][2]int64{}
	for _, row := range types {
		require.NotContains(t, typed, row.ProjectID.String+"/"+row.ShareTypeID.String+"/"+row.Resource.String)
		typed[row.ProjectID.String+"/"+row.ShareTypeID.String+"/"+row.Resource.String] = [2]int64{row.InUse, row.Reserved}
		require.Contains(t, []string{"gold", "silver"}, row.ShareTypeName)
	}
	expectedTypes := map[string][2]int64{}
	for i, resource := range strings.Fields("shares gigabytes snapshots snapshot_gigabytes share_replicas replica_gigabytes") {
		expectedTypes["project-a/type-a/"+resource] = [2]int64{int64(i + 4), 2}
	}
	expectedTypes["project-a/type-b/shares"] = [2]int64{8, 3}
	expectedTypes["type-usage-only/type-a/snapshots"] = [2]int64{1, 2}
	require.Equal(t, expectedTypes, typed)
	limits, err := r.GetShareTypeLimits(ctx)
	require.NoError(t, err)
	require.Len(t, limits, 4)
	users, err := r.GetUserLimits(ctx)
	require.NoError(t, err)
	require.Len(t, users, 3)
	defaults, err := r.GetDefaultLimits(ctx)
	require.NoError(t, err)
	require.Len(t, defaults, 3)
	projectLimits, err := r.GetProjectLimits(ctx)
	require.NoError(t, err)
	require.Len(t, projectLimits, 12)
	for _, row := range projectLimits {
		require.True(t, row.ProjectID.Valid)
		require.Equal(t, "project-a", row.ProjectID.String)
		switch row.Resource.String {
		case "gigabytes":
			require.False(t, row.HardLimit.Valid)
		case "snapshots":
			require.Equal(t, sql.NullInt32{Int32: -1, Valid: true}, row.HardLimit)
		case "share_networks":
			require.Equal(t, sql.NullInt32{Valid: true}, row.HardLimit)
		}
	}
	_, err = db.ExecContext(ctx, "DROP TABLE quota_usages")
	require.NoError(t, err)
	_, err = r.GetProjectUsage(ctx)
	require.Error(t, err)
	_, err = r.GetShareTypeUsage(ctx)
	require.Error(t, err)
}
