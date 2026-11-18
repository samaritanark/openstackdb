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

func TestQuotaQueriesPropagateErrors(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		columns     []string
		read        func(context.Context, *Queries) (any, error)
	}{
		{"defaults", QuotaClassGetDefault, []string{"resource", "hard_limit"}, func(ctx context.Context, q *Queries) (any, error) { return q.QuotaClassGetDefault(ctx) }},
		{"projects", QuotaGetAllByProject, []string{"project_id", "resource", "hard_limit"}, func(ctx context.Context, q *Queries) (any, error) { return q.QuotaGetAllByProject(ctx) }},
		{"users", QuotaGetAllByProjectAndUser, []string{"project_id", "user_id", "resource", "hard_limit"}, func(ctx context.Context, q *Queries) (any, error) { return q.QuotaGetAllByProjectAndUser(ctx) }},
		{"share types", QuotaGetAllByProjectAndShareType, []string{"project_id", "share_type_id", "share_type_name", "resource", "hard_limit"}, func(ctx context.Context, q *Queries) (any, error) { return q.QuotaGetAllByProjectAndShareType(ctx) }},
		{"project usage", QuotaUsageGetAllByProject, []string{"project_id", "user_id", "resource", "in_use", "reserved"}, func(ctx context.Context, q *Queries) (any, error) { return q.QuotaUsageGetAllByProject(ctx) }},
		{"share type usage", QuotaUsageGetAllByProjectAndShareType, []string{"project_id", "share_type_id", "share_type_name", "user_id", "resource", "in_use", "reserved"}, func(ctx context.Context, q *Queries) (any, error) {
			return q.QuotaUsageGetAllByProjectAndShareType(ctx)
		}},
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
						query.WillReturnRows(rows.AddRow(values...)).RowsWillBeClosed()
					case "iteration error":
						// Fail before scanning; the generated query must return rows.Err.
						values := make([]driver.Value, len(tc.columns))
						for i, column := range tc.columns {
							switch column {
							case "hard_limit", "in_use", "reserved":
								values[i] = 1
							default:
								values[i] = "x"
							}
						}
						rows.AddRow(values...).RowError(0, errors.New("iteration failed"))
						query.WillReturnRows(rows).RowsWillBeClosed()
					default:
						query.WillReturnRows(rows).RowsWillBeClosed()
					}
				}
				got, err := tc.read(ctx, New(db))
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

// Usage rows are returned raw: NULL amounts stay NULL rather than becoming zero,
// and a deleted share type surfaces as a NULL name instead of a dropped row.
func TestQuotaUsageScansRawValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	q := New(db)
	ctx := context.Background()
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	num := func(n int32) sql.NullInt32 { return sql.NullInt32{Int32: n, Valid: true} }

	mock.ExpectQuery(regexp.QuoteMeta(QuotaUsageGetAllByProject)).WillReturnRows(
		sqlmock.NewRows([]string{"project_id", "user_id", "resource", "in_use", "reserved"}).
			AddRow("p", "u", "shares", 3, 1).
			AddRow("p", nil, "shares", nil, nil))
	usage, err := q.QuotaUsageGetAllByProject(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaUsageGetAllByProjectRow{
		{ProjectID: str("p"), UserID: str("u"), Resource: str("shares"), InUse: num(3), Reserved: num(1)},
		{ProjectID: str("p"), Resource: str("shares")},
	}, usage)

	mock.ExpectQuery(regexp.QuoteMeta(QuotaUsageGetAllByProjectAndShareType)).WillReturnRows(
		sqlmock.NewRows([]string{"project_id", "share_type_id", "share_type_name", "user_id", "resource", "in_use", "reserved"}).
			AddRow("p", "t", "gold", nil, "shares", 8, 2).
			AddRow("p", "t-deleted", nil, nil, "shares", 5, 0))
	typed, err := q.QuotaUsageGetAllByProjectAndShareType(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaUsageGetAllByProjectAndShareTypeRow{
		{ProjectID: str("p"), ShareTypeID: str("t"), ShareTypeName: str("gold"), Resource: str("shares"), InUse: num(8), Reserved: num(2)},
		{ProjectID: str("p"), ShareTypeID: str("t-deleted"), Resource: str("shares"), InUse: num(5), Reserved: num(0)},
	}, typed)

	mock.ExpectQuery(regexp.QuoteMeta(QuotaClassGetDefault)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "hard_limit"}).AddRow("shares", nil))
	defaults, err := q.QuotaClassGetDefault(ctx)
	require.NoError(t, err)
	require.Equal(t, []QuotaClassGetDefaultRow{{Resource: str("shares")}}, defaults)
	require.NoError(t, mock.ExpectationsWereMet())
}
