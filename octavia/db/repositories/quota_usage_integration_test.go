//go:build integration

package octavia

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
)

// Runs only against the disposable MariaDB container created by the test helper.
func TestQuotaUsageMariaDB(t *testing.T) {
	db := testutil.NewMySQLContainer(t, "octavia", "../../../sql/octavia/schema.sql")
	fixture, err := os.ReadFile("../../../tests/fixtures/octavia_quota_usage.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, db, string(fixture))
	rows, err := NewQuotaRepository(db).GetUsage(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 14)

	expected := map[string]int64{}
	for index, resource := range strings.Fields("load_balancer listener pool member health_monitor l7policy l7rule") {
		expected["project-a/"+resource] = int64(index + 4)
		expected["project-b/"+resource] = 1
	}
	got := map[string]int64{}
	for _, row := range rows {
		require.True(t, row.ProjectID.Valid)
		key := row.ProjectID.String + "/" + row.Resource
		require.NotContains(t, got, key)
		got[key] = row.InUse
	}
	require.Equal(t, expected, got)

	t.Run("nullable limits", func(t *testing.T) {
		// QuotaGetAll returns stored limits, including an empty project key.
		// Owner filtering applies to resource counts, not the quota table read.
		testutil.SeedSQL(t, db, "INSERT INTO quotas (project_id) VALUES ('')")
		limits, err := NewQuotaRepository(db).GetAll(context.Background())
		require.NoError(t, err)
		require.Len(t, limits, 2)
		byProject := map[string]QuotaGetAllRow{}
		for _, row := range limits {
			byProject[row.ProjectID] = row
		}
		limit := func(n int32) sql.NullInt32 { return sql.NullInt32{Int32: n, Valid: true} }
		require.Equal(t, map[string]QuotaGetAllRow{
			"": {ProjectID: ""},
			"project-a": {ProjectID: "project-a", Listener: limit(-1), LoadBalancer: limit(0),
				Member: limit(0), Pool: limit(17), L7rule: limit(23)},
		}, byProject)
	})

	// The shared fixture includes DELETED rows and both NULL and empty project
	// IDs. The older per-project queries must agree with rebuilt quota usage.
	for _, tc := range []struct {
		name  string
		query string
		count int64
	}{
		{"load balancers", LoadBalancerCountByProject, 4},
		{"listeners", ListenerCountByProject, 5},
		{"pools", PoolCountByProject, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.QueryContext(context.Background(), tc.query)
			require.NoError(t, err)
			defer rows.Close()
			counts := map[string]int64{}
			for rows.Next() {
				var projectID sql.NullString
				var count int64
				require.NoError(t, rows.Scan(&projectID, &count))
				require.True(t, projectID.Valid)
				require.NotEmpty(t, projectID.String)
				counts[projectID.String] = count
			}
			require.NoError(t, rows.Err())
			require.Equal(t, map[string]int64{"project-a": tc.count, "project-b": 1}, counts)
		})
	}
}
