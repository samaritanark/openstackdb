//go:build integration

package tests

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
	octavia "github.com/vexxhost/openstackdb/octavia/db/repositories"
)

// The fixture includes DELETED rows, NULL and empty project IDs, and cached
// in_use_* counters that both match and drift from the recount.
func TestOctaviaQuotas(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewMySQLContainer(t, "octavia-quotas", "../sql/octavia/schema.sql")
	fixture, err := os.ReadFile("fixtures/octavia_quota_usage.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, db, string(fixture), "INSERT INTO quotas (project_id) VALUES ('')")
	r := octavia.NewQuotaRepository(db)

	usage, err := r.GetUsage(ctx)
	require.NoError(t, err)
	expected := map[string]int64{}
	for index, resource := range strings.Fields("load_balancer listener pool member health_monitor l7policy l7rule") {
		expected["project-a/"+resource] = int64(index + 4)
		expected["project-b/"+resource] = 1
	}
	got := map[string]int64{}
	for _, row := range usage {
		require.True(t, row.ProjectID.Valid)
		key := row.ProjectID.String + "/" + row.Resource
		require.NotContains(t, got, key)
		got[key] = row.InUse
	}
	require.Equal(t, expected, got)

	// Limits and cached counters come back as stored. The empty-string row is
	// excluded, matching GetUsage, because Octavia rejects empty project IDs.
	quotas, err := r.GetAll(ctx)
	require.NoError(t, err)
	n := func(v int32) sql.NullInt32 { return sql.NullInt32{Int32: v, Valid: true} }
	require.Equal(t, []octavia.Quota{{
		ProjectID: "project-a",
		Listener:  n(-1), LoadBalancer: n(0), Member: n(0), Pool: n(17), L7rule: n(23),
		InUseLoadBalancer: n(4), InUseMember: n(999),
	}}, quotas)

	// Drift is visible by comparing the cached counter with the recount.
	require.Equal(t, int64(quotas[0].InUseLoadBalancer.Int32), got["project-a/load_balancer"])
	require.NotEqual(t, int64(quotas[0].InUseMember.Int32), got["project-a/member"])
}
