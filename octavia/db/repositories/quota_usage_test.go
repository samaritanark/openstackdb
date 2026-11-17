package octavia

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

func TestQuotaUsageRepository(t *testing.T) {
	for _, scenario := range []string{"rows", "empty", "query error", "scan error", "iteration error"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			query := mock.ExpectQuery(regexp.QuoteMeta(QuotaUsageGetAll))
			rows := sqlmock.NewRows([]string{"project_id", "resource", "in_use"})
			readErr := errors.New("read failed")
			switch scenario {
			case "rows":
				rows.AddRow("project-a", "member", int64(1)<<33)
				query.WillReturnRows(rows).RowsWillBeClosed()
			case "empty":
				query.WillReturnRows(rows).RowsWillBeClosed()
			case "query error":
				query.WillReturnError(sql.ErrConnDone)
			case "scan error":
				rows.AddRow("project-a", "member", "invalid integer")
				query.WillReturnRows(rows).RowsWillBeClosed()
			case "iteration error":
				rows.AddRow("project-a", "member", 4).RowError(0, readErr)
				query.WillReturnRows(rows).RowsWillBeClosed()
			}
			got, err := NewQuotaRepository(db).GetUsage(context.Background())
			if scenario == "rows" {
				require.NoError(t, err)
				require.Equal(t, []QuotaUsageGetAllRow{{ProjectID: sql.NullString{String: "project-a", Valid: true}, Resource: "member", InUse: int64(1) << 33}}, got)
			} else if scenario == "empty" {
				require.NoError(t, err)
				require.Empty(t, got)
			} else {
				require.Error(t, err)
				if scenario == "query error" {
					require.ErrorIs(t, err, sql.ErrConnDone)
				} else if scenario == "iteration error" {
					require.ErrorIs(t, err, readErr)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestQuotaLimitsAndCountsRepository(t *testing.T) {
	project := sql.NullString{String: "project-a", Valid: true}
	limit := func(n int32) sql.NullInt32 { return sql.NullInt32{Int32: n, Valid: true} }
	for _, tc := range []struct {
		name    string
		query   string
		columns []string
		values  []driver.Value
		want    any
		call    func(*QuotaRepository) (any, error)
	}{
		{
			name: "limits", query: QuotaGetAll,
			columns: []string{"project_id", "health_monitor", "listener", "load_balancer", "member", "pool", "l7policy", "l7rule"},
			values:  []driver.Value{"project-a", nil, -1, 0, 0, 17, nil, 23},
			want: []QuotaGetAllRow{{ProjectID: "project-a", Listener: limit(-1), LoadBalancer: limit(0),
				Member: limit(0), Pool: limit(17), L7rule: limit(23)}},
			call: func(r *QuotaRepository) (any, error) { return r.GetAll(context.Background()) },
		},
		{
			name: "load balancers", query: LoadBalancerCountByProject,
			columns: []string{"project_id", "count"}, values: []driver.Value{"project-a", int64(1) << 33},
			want: []LoadBalancerCountByProjectRow{{ProjectID: project, Count: int64(1) << 33}},
			call: func(r *QuotaRepository) (any, error) { return r.LoadBalancerCountByProject(context.Background()) },
		},
		{
			name: "listeners", query: ListenerCountByProject,
			columns: []string{"project_id", "count"}, values: []driver.Value{"project-a", int64(1) << 33},
			want: []ListenerCountByProjectRow{{ProjectID: project, Count: int64(1) << 33}},
			call: func(r *QuotaRepository) (any, error) { return r.ListenerCountByProject(context.Background()) },
		},
		{
			name: "pools", query: PoolCountByProject,
			columns: []string{"project_id", "count"}, values: []driver.Value{"project-a", int64(1) << 33},
			want: []PoolCountByProjectRow{{ProjectID: project, Count: int64(1) << 33}},
			call: func(r *QuotaRepository) (any, error) { return r.PoolCountByProject(context.Background()) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, scenario := range []string{"rows", "empty", "query error", "scan error", "iteration error"} {
				t.Run(scenario, func(t *testing.T) {
					db, mock, err := sqlmock.New()
					require.NoError(t, err)
					defer db.Close()
					query := mock.ExpectQuery(regexp.QuoteMeta(tc.query))
					rows := sqlmock.NewRows(tc.columns)
					readErr := errors.New("read failed")
					switch scenario {
					case "rows":
						rows.AddRow(tc.values...)
					case "query error":
						query.WillReturnError(sql.ErrConnDone)
					case "scan error":
						values := append([]driver.Value(nil), tc.values...)
						values[1] = "invalid integer"
						rows.AddRow(values...)
					case "iteration error":
						rows.AddRow(tc.values...).RowError(0, readErr)
					}
					if scenario != "query error" {
						query.WillReturnRows(rows).RowsWillBeClosed()
					}
					got, err := tc.call(NewQuotaRepository(db))
					switch scenario {
					case "rows":
						require.NoError(t, err)
						require.Equal(t, tc.want, got)
					case "empty":
						require.NoError(t, err)
						require.Empty(t, got)
					case "query error":
						require.ErrorIs(t, err, sql.ErrConnDone)
					case "scan error":
						require.ErrorContains(t, err, "invalid integer")
					case "iteration error":
						require.ErrorIs(t, err, readErr)
					}
					require.NoError(t, mock.ExpectationsWereMet())
				})
			}
		})
	}
}
