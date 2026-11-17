package manila

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQuotaRepository(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		columns     []string
		read        func(context.Context, *QuotaRepository) (any, error)
	}{
		{"defaults", QuotaGetDefaultLimits, []string{"resource", "hard_limit"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetDefaultLimits(ctx) }},
		{"projects", QuotaGetProjectLimits, []string{"project_id", "resource", "hard_limit"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetProjectLimits(ctx) }},
		{"users", QuotaGetUserLimits, []string{"project_id", "user_id", "resource", "hard_limit"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetUserLimits(ctx) }},
		{"share types", QuotaGetShareTypeLimits, []string{"project_id", "share_type_id", "share_type_name", "resource", "hard_limit"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetShareTypeLimits(ctx) }},
		{"project usage", QuotaUsageGetProjectTotals, []string{"project_id", "resource", "in_use", "reserved"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetProjectUsage(ctx) }},
		{"share type usage", QuotaUsageGetShareTypeTotals, []string{"project_id", "share_type_id", "share_type_name", "resource", "in_use", "reserved"}, func(ctx context.Context, r *QuotaRepository) (any, error) { return r.GetShareTypeUsage(ctx) }},
	} {
		for _, scenario := range []string{"empty", "query error", "scan error", "iteration error", "canceled"} {
			t.Run(tc.name+"/"+scenario, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer func() { _ = db.Close() }()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if scenario == "canceled" {
					cancel()
				} else {
					query := mock.ExpectQuery(regexp.QuoteMeta(tc.query))
					rows := sqlmock.NewRows(tc.columns)
					switch scenario {
					case "query error":
						query.WillReturnError(sql.ErrConnDone)
					case "scan error":
						values := make([]driver.Value, len(tc.columns))
						for i := range values {
							values[i] = "invalid integer"
						}
						rows.AddRow(values...)
						query.WillReturnRows(rows).RowsWillBeClosed()
					case "iteration error":
						// Fail before scanning; the generated query must return rows.Err.
						switch len(tc.columns) {
						case 2:
							rows.AddRow("shares", 1)
						case 3:
							rows.AddRow("p", "shares", 1)
						case 4:
							rows.AddRow("p", "u", "shares", 1)
						case 5:
							rows.AddRow("p", "t", "gold", "shares", 1)
						case 6:
							rows.AddRow("p", "t", "gold", "shares", 1, 2)
						}
						rows.RowError(0, errors.New("iteration failed"))
						query.WillReturnRows(rows).RowsWillBeClosed()
					default:
						query.WillReturnRows(rows).RowsWillBeClosed()
					}
				}
				got, err := tc.read(ctx, NewQuotaRepository(db))
				if scenario == "empty" {
					require.NoError(t, err)
					require.Empty(t, got)
				} else {
					require.Error(t, err)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestQuotaRepositoryScansValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := NewQuotaRepository(db)
	ctx := context.Background()
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetDefaultLimits)).WillReturnRows(sqlmock.NewRows([]string{"resource", "hard_limit"}).AddRow("shares", nil))
	defaults, err := r.GetDefaultLimits(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaGetDefaultLimitsRow{{Resource: sql.NullString{String: "shares", Valid: true}}}, defaults)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaUsageGetProjectTotals)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).AddRow("p", "gigabytes", int64(4000000000), 3))
	usage, err := r.GetProjectUsage(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaUsageGetProjectTotalsRow{{ProjectID: sql.NullString{String: "p", Valid: true}, Resource: sql.NullString{String: "gigabytes", Valid: true}, InUse: 4000000000, Reserved: 3}}, usage)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaUsageGetShareTypeTotals)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "share_type_id", "share_type_name", "resource", "in_use", "reserved"}).AddRow("p", "t", "gold", "shares", 8, 2))
	types, err := r.GetShareTypeUsage(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaUsageGetShareTypeTotalsRow{{ProjectID: sql.NullString{String: "p", Valid: true}, ShareTypeID: sql.NullString{String: "t", Valid: true}, ShareTypeName: "gold", Resource: sql.NullString{String: "shares", Valid: true}, InUse: 8, Reserved: 2}}, types)
	require.NoError(t, mock.ExpectationsWereMet())
}
