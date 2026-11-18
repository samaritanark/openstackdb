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

func TestQuotaLimitsRepository(t *testing.T) {
	n := func(v int32) sql.NullInt32 { return sql.NullInt32{Int32: v, Valid: true} }
	columns := []string{"project_id", "health_monitor", "listener", "load_balancer", "member", "pool", "l7policy", "l7rule",
		"in_use_health_monitor", "in_use_listener", "in_use_load_balancer", "in_use_member", "in_use_pool", "in_use_l7policy", "in_use_l7rule"}
	values := []driver.Value{"project-a", nil, -1, 0, 0, 17, nil, 23, nil, nil, 4, 999, nil, nil, nil}
	want := []Quota{{ProjectID: "project-a", Listener: n(-1), LoadBalancer: n(0), Member: n(0), Pool: n(17), L7rule: n(23),
		InUseLoadBalancer: n(4), InUseMember: n(999)}}
	for _, scenario := range []string{"rows", "empty", "query error", "scan error", "iteration error"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			query := mock.ExpectQuery(regexp.QuoteMeta(QuotaGetAll))
			rows := sqlmock.NewRows(columns)
			readErr := errors.New("read failed")
			switch scenario {
			case "rows":
				rows.AddRow(values...)
			case "query error":
				query.WillReturnError(sql.ErrConnDone)
			case "scan error":
				bad := append([]driver.Value(nil), values...)
				bad[11] = "invalid integer"
				rows.AddRow(bad...)
			case "iteration error":
				rows.AddRow(values...).RowError(0, readErr)
			}
			if scenario != "query error" {
				query.WillReturnRows(rows).RowsWillBeClosed()
			}
			got, err := NewQuotaRepository(db).GetAll(context.Background())
			switch scenario {
			case "rows":
				require.NoError(t, err)
				require.Equal(t, want, got)
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
}
