package manila

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestShareInventoryScansValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(regexp.QuoteMeta(ShareGetInventory)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "project_id", "size", "share_proto", "status", "share_type", "share_type_name", "availability_zone"}).
			AddRow("no-instance", nil, nil, nil, nil, nil, "", "", "").
			AddRow("share", "name", "project", 10, "NFS", "available", "type", "gold", "zone"),
	).RowsWillBeClosed()
	rows, err := New(db).ShareGetInventory(context.Background())
	require.NoError(t, err)
	require.Equal(t, []ShareGetInventoryRow{
		{ID: "no-instance"},
		{
			ID: "share", Name: sql.NullString{String: "name", Valid: true},
			ProjectID:  sql.NullString{String: "project", Valid: true},
			Size:       sql.NullInt32{Int32: 10, Valid: true},
			ShareProto: sql.NullString{String: "NFS", Valid: true},
			Status:     sql.NullString{String: "available", Valid: true},
			ShareType:  "type", ShareTypeName: "gold", AvailabilityZone: "zone",
		},
	}, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestShareInventoryErrors(t *testing.T) {
	for _, scenario := range []string{"empty", "query", "scan", "iteration", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			} else {
				query := mock.ExpectQuery(regexp.QuoteMeta(ShareGetInventory))
				rows := sqlmock.NewRows([]string{"id", "name", "project_id", "size", "share_proto", "status", "share_type", "share_type_name", "availability_zone"})
				switch scenario {
				case "query":
					query.WillReturnError(sql.ErrConnDone)
				case "scan":
					query.WillReturnRows(rows.AddRow("s", nil, nil, "invalid integer", nil, nil, "", "", "")).RowsWillBeClosed()
				case "iteration":
					query.WillReturnRows(rows.AddRow("s", nil, nil, nil, nil, nil, "", "", "").RowError(0, sql.ErrConnDone)).RowsWillBeClosed()
				default:
					query.WillReturnRows(rows).RowsWillBeClosed()
				}
			}
			rows, err := New(db).ShareGetInventory(ctx)
			switch scenario {
			case "empty":
				require.NoError(t, err)
				require.Empty(t, rows)
			case "query", "iteration":
				require.ErrorIs(t, err, sql.ErrConnDone)
			case "canceled":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.Error(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
