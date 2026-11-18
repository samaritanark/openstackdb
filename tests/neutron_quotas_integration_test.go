//go:build integration

package tests

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
	neutron "github.com/vexxhost/openstackdb/neutron/db"
)

func TestNeutronReservationsForResources(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewMySQLContainer(t, "neutron-reservations", "../sql/neutron/schema.sql")

	// Expirations are relative to the database clock, which is what the query
	// compares against, so the test does not depend on the runner's clock.
	testutil.SeedSQL(t, db,
		"INSERT INTO reservations VALUES ('active-a', 'a', UTC_TIMESTAMP() + INTERVAL 1 HOUR)",
		"INSERT INTO reservations VALUES ('active-empty', '', UTC_TIMESTAMP() + INTERVAL 1 HOUR)",
		"INSERT INTO reservations VALUES ('active-null-owner', NULL, UTC_TIMESTAMP() + INTERVAL 1 HOUR)",
		"INSERT INTO reservations VALUES ('expired', 'a', UTC_TIMESTAMP() - INTERVAL 1 HOUR)",
		"INSERT INTO reservations VALUES ('null-expiration', 'a', NULL)",
		"INSERT INTO resourcedeltas VALUES ('port', 'active-a', 3)",
		"INSERT INTO resourcedeltas VALUES ('network', 'active-a', -2)",
		"INSERT INTO resourcedeltas VALUES ('router', 'active-a', 1)",
		"INSERT INTO resourcedeltas VALUES ('port', 'active-empty', NULL)",
		"INSERT INTO resourcedeltas VALUES ('port', 'active-null-owner', 5)",
		"INSERT INTO resourcedeltas VALUES ('port', 'expired', 7)",
		"INSERT INTO resourcedeltas VALUES ('port', 'null-expiration', 9)",
	)

	q := neutron.New(db)
	rows, err := q.GetReservationsForResources(ctx, []string{"port", "network"})
	require.NoError(t, err)

	type delta struct {
		ID, Project, Resource string
		Amount                sql.NullInt32
	}
	var got []delta
	for _, r := range rows {
		require.True(t, r.ProjectID.Valid)
		require.True(t, r.Expiration.Valid)
		got = append(got, delta{r.ReservationID, r.ProjectID.String, r.Resource, r.Amount})
	}
	// Ordered by project, resource, reservation. Expired, NULL-expiration,
	// NULL-owner and unrequested-resource rows are excluded; empty-string
	// owners are kept, matching GetQuotas and GetResourceCountsByProject.
	require.Equal(t, []delta{
		{"active-empty", "", "port", sql.NullInt32{}},
		{"active-a", "a", "network", sql.NullInt32{Int32: -2, Valid: true}},
		{"active-a", "a", "port", sql.NullInt32{Int32: 3, Valid: true}},
	}, got)

	rows, err = q.GetReservationsForResources(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestNeutronQuotaOwnersAndOrdering(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewMySQLContainer(t, "neutron-quotas", "../sql/neutron/schema.sql")

	// Each table owns its counts without requiring parent rows or quota overrides.
	inserts := map[string]string{
		"floatingip":          "INSERT INTO floatingips (id, project_id, floating_ip_address, floating_network_id, floating_port_id, status, standard_attr_id) VALUES (?, ?, '192.0.2.1', 'parent-network', 'parent-port', 'DOWN', 1)",
		"network":             "INSERT INTO networks (id, project_id, status, standard_attr_id) VALUES (?, ?, 'DOWN', 1)",
		"port":                "INSERT INTO ports (id, project_id, network_id, mac_address, admin_state_up, status, device_id, device_owner, standard_attr_id) VALUES (?, ?, 'parent-network', '00:00:00:00:00:01', 0, 'DOWN', '', '', 1)",
		"router":              "INSERT INTO routers (id, project_id, status, standard_attr_id) VALUES (?, ?, 'DOWN', 1)",
		"security_group":      "INSERT INTO securitygroups (id, project_id, standard_attr_id) VALUES (?, ?, 1)",
		"security_group_rule": "INSERT INTO securitygrouprules (id, project_id, security_group_id, standard_attr_id) VALUES (?, ?, 'parent-group', 1)",
		"subnet":              "INSERT INTO subnets (id, project_id, network_id, ip_version, cidr, standard_attr_id) VALUES (?, ?, 'parent-network', 4, '192.0.2.0/24', 1)",
		"rbac_policy":         "INSERT INTO networkrbacs (id, project_id, object_id, target_project, action) VALUES (?, ?, 'parent-network', 'target-owner', 'access_as_shared')",
		"subnetpool":          "INSERT INTO subnetpools (id, project_id, ip_version, default_prefixlen, min_prefixlen, max_prefixlen, standard_attr_id) VALUES (?, ?, 4, 24, 8, 32, 1)",
	}
	var expected []neutron.GetResourceCountsByProjectRow
	for resource, insert := range inserts {
		for j, owner := range []any{"a", "a", "b", "", nil} {
			_, err := db.ExecContext(ctx, insert, fmt.Sprintf("%s-%d", resource, j), owner)
			require.NoError(t, err)
		}
		expected = append(expected,
			neutron.GetResourceCountsByProjectRow{ProjectID: sql.NullString{String: "a", Valid: true}, Resource: resource, Cnt: 2},
			neutron.GetResourceCountsByProjectRow{ProjectID: sql.NullString{String: "b", Valid: true}, Resource: resource, Cnt: 1},
			// Empty-string owners (for example L3 HA networks and ports) are kept.
			neutron.GetResourceCountsByProjectRow{ProjectID: sql.NullString{Valid: true}, Resource: resource, Cnt: 1})
	}
	q := neutron.New(db)
	usage, err := q.GetResourceCountsByProject(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, expected, usage)

	// Duplicate project/resource rows are kept and ordered by id, so callers
	// that collapse duplicates see a stable result.
	testutil.SeedSQL(t, db,
		"INSERT INTO quotas VALUES ('q3', 'b', 'port', 10)",
		"INSERT INTO quotas VALUES ('q2', 'a', 'port', 42)",
		"INSERT INTO quotas VALUES ('q1', 'a', 'port', -1)",
		"INSERT INTO quotas VALUES ('q4', 'a', 'network', NULL)",
		"INSERT INTO quotas VALUES ('q5', '', 'port', 0)",
		"INSERT INTO quotas VALUES ('q6', NULL, 'port', 5)",
	)
	limits, err := q.GetQuotas(ctx)
	require.NoError(t, err)
	port := sql.NullString{String: "port", Valid: true}
	require.Equal(t, []neutron.GetQuotasRow{
		{ProjectID: sql.NullString{Valid: true}, Resource: port, Limit: sql.NullInt32{Valid: true}},
		{ProjectID: sql.NullString{String: "a", Valid: true}, Resource: sql.NullString{String: "network", Valid: true}},
		{ProjectID: sql.NullString{String: "a", Valid: true}, Resource: port, Limit: sql.NullInt32{Int32: -1, Valid: true}},
		{ProjectID: sql.NullString{String: "a", Valid: true}, Resource: port, Limit: sql.NullInt32{Int32: 42, Valid: true}},
		{ProjectID: sql.NullString{String: "b", Valid: true}, Resource: port, Limit: sql.NullInt32{Int32: 10, Valid: true}},
	}, limits)
}
