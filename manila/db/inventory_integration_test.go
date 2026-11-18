//go:build integration

package manila

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vexxhost/openstackdb/internal/testutil"
)

// Uses only a disposable MariaDB container; never reads a production DSN.
func TestShareInventoryMariaDB(t *testing.T) {
	db := testutil.NewMySQLContainer(t, "manila-inventory", "../../sql/manila/prereqs.sql", "../../sql/manila/schema.sql")
	q := New(db)
	rows, err := q.ShareGetInventory(context.Background())
	require.NoError(t, err)
	require.Empty(t, rows)
	fixture, err := os.ReadFile("../../tests/fixtures/manila_inventory.sql")
	require.NoError(t, err)
	testutil.SeedSQL(t, db, string(fixture))
	rows, err = q.ShareGetInventory(context.Background())
	require.NoError(t, err)
	expectedJSON, err := os.ReadFile("../../tests/fixtures/manila_inventory_expected.json")
	require.NoError(t, err)
	var expected map[string][]any
	require.NoError(t, json.Unmarshal(expectedJSON, &expected))
	got := map[string][]any{}
	nullable := func(s sql.NullString) any {
		if !s.Valid {
			return nil
		}
		return s.String
	}
	var capacity int32
	var ids []string
	for _, row := range rows {
		require.NotContains(t, got, row.ID)
		ids = append(ids, row.ID)
		got[row.ID] = []any{row.ID, nullable(row.Name), nullable(row.ProjectID), float64(row.Size.Int32), nullable(row.ShareProto), nullable(row.Status), row.ShareType, row.ShareTypeName, row.AvailabilityZone}
		capacity += row.Size.Int32
	}
	require.Equal(t, expected, got)
	require.EqualValues(t, 10*len(expected), capacity)
	// All fixture created_at values are NULL, so IDs break the ordering tie.
	require.True(t, sort.StringsAreSorted(ids))
	legacy, err := q.ShareGetAllWithInstances(context.Background())
	require.NoError(t, err)
	require.Len(t, legacy, 34)
	counts := map[string]int{}
	for _, row := range legacy {
		counts[row.ID]++
	}
	require.Len(t, counts, len(expected))
	for id := range expected {
		require.Positive(t, counts[id], id)
	}
	require.Equal(t, 2, counts["identical"])
	require.Equal(t, 3, counts["reverting-promotion"])
	require.Equal(t, 2, counts["snapshot-revert"])
	require.Equal(t, 1, counts["zero"])
	require.Equal(t, 1, counts["deleted-only"])
	require.Equal(t, 1, counts["soft-retained"])
	require.Zero(t, counts["deleted-share"])
}
