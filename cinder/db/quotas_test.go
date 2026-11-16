package cinder

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQuotaRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := NewQuotaRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetActiveLimits)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "resource", "hard_limit"}).AddRow("p", "volumes", nil))
	limits, err := r.GetProjectLimits(context.Background())
	require.NoError(t, err)
	require.False(t, limits[0].HardLimit.Valid)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetActiveUsage)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).AddRow("usage-only", "backups", 7, 4))
	usage, err := r.GetProjectUsage(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 7, usage[0].InUse)
	require.EqualValues(t, 4, usage[0].Reserved)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetDefaultClassLimits)).WillReturnRows(sqlmock.NewRows([]string{"resource", "hard_limit"}).AddRow("volumes", 42))
	defaults, err := r.GetDefaultLimits(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, defaults[0].HardLimit.Int32)
	mock.ExpectQuery(regexp.QuoteMeta(VolumeTypeGetAllCurrent)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("t", "fast_ssd"))
	types, err := r.GetVolumeTypes(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fast_ssd", types[0].Name.String)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuotaRepositoryErrorsAndDeadline(t *testing.T) {
	for _, op := range []struct {
		query string
		read  func(*QuotaRepository, context.Context) error
	}{
		{QuotaGetActiveLimits, func(r *QuotaRepository, c context.Context) error { _, err := r.GetProjectLimits(c); return err }},
		{QuotaGetActiveUsage, func(r *QuotaRepository, c context.Context) error { _, err := r.GetProjectUsage(c); return err }},
		{QuotaGetDefaultClassLimits, func(r *QuotaRepository, c context.Context) error { _, err := r.GetDefaultLimits(c); return err }},
		{VolumeTypeGetAllCurrent, func(r *QuotaRepository, c context.Context) error { _, err := r.GetVolumeTypes(c); return err }},
	} {
		t.Run(op.query[:20], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			r := NewQuotaRepository(db)
			failure := errors.New("query failure")
			mock.ExpectQuery(regexp.QuoteMeta(op.query)).WillReturnError(failure)
			require.ErrorIs(t, op.read(r, context.Background()), failure)
			mock.ExpectQuery(regexp.QuoteMeta(op.query)).WillDelayFor(time.Second).WillReturnError(failure)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			start := time.Now()
			require.Error(t, op.read(r, ctx))
			require.Less(t, time.Since(start), 500*time.Millisecond)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestQuotaRepositoryRawRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := NewQuotaRepository(db)
	s := func(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
	n := func(v int32) sql.NullInt32 { return sql.NullInt32{Int32: v, Valid: true} }
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetActiveLimits)).WillReturnRows(
		sqlmock.NewRows([]string{"project_id", "resource", "hard_limit"}).
			AddRow(nil, "volumes", nil).AddRow("p", "volumes", 0).
			AddRow("p", "volumes", -1).AddRow("", "gigabytes_fast_ssd", math.MaxInt32))
	limits, err := r.GetProjectLimits(context.Background())
	require.NoError(t, err)
	require.ElementsMatch(t, []QuotaGetActiveLimitsRow{
		{sql.NullString{}, "volumes", sql.NullInt32{}},
		{s("p"), "volumes", n(0)}, {s("p"), "volumes", n(-1)},
		{s(""), "gigabytes_fast_ssd", n(math.MaxInt32)},
	}, limits)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetActiveUsage)).WillReturnRows(
		sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).
			AddRow(nil, nil, math.MinInt32, math.MaxInt32).
			AddRow("usage-only", "volumes", 2, 3).AddRow("usage-only", "volumes", 8, 9))
	usage, err := r.GetProjectUsage(context.Background())
	require.NoError(t, err)
	require.ElementsMatch(t, []QuotaGetActiveUsageRow{
		{sql.NullString{}, sql.NullString{}, math.MinInt32, math.MaxInt32},
		{s("usage-only"), s("volumes"), 2, 3}, {s("usage-only"), s("volumes"), 8, 9},
	}, usage)
	mock.ExpectQuery(regexp.QuoteMeta(QuotaGetDefaultClassLimits)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "hard_limit"}).
			AddRow(nil, nil).AddRow("volumes", 0).AddRow("volumes", -1))
	defaults, err := r.GetDefaultLimits(context.Background())
	require.NoError(t, err)
	require.ElementsMatch(t, []QuotaGetDefaultClassLimitsRow{
		{sql.NullString{}, sql.NullInt32{}}, {s("volumes"), n(0)}, {s("volumes"), n(-1)},
	}, defaults)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuotaRepositoryReadFailures(t *testing.T) {
	for _, op := range []struct {
		name    string
		query   string
		columns []string
		read    func(*QuotaRepository, context.Context) error
	}{
		{"limits", QuotaGetActiveLimits, []string{"project_id", "resource", "hard_limit"}, func(r *QuotaRepository, c context.Context) error { _, err := r.GetProjectLimits(c); return err }},
		{"usage", QuotaGetActiveUsage, []string{"project_id", "resource", "in_use", "reserved"}, func(r *QuotaRepository, c context.Context) error { _, err := r.GetProjectUsage(c); return err }},
		{"defaults", QuotaGetDefaultClassLimits, []string{"resource", "hard_limit"}, func(r *QuotaRepository, c context.Context) error { _, err := r.GetDefaultLimits(c); return err }},
		{"types", VolumeTypeGetAllCurrent, []string{"id", "name"}, func(r *QuotaRepository, c context.Context) error { _, err := r.GetVolumeTypes(c); return err }},
	} {
		t.Run(op.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			r := NewQuotaRepository(db)
			failure := errors.New("read failure")
			// An iteration failure must reach the caller, including after a
			// successful query. All result sets must be closed on errors.
			rows := sqlmock.NewRows(op.columns)
			if len(op.columns) == 4 {
				rows.AddRow("p", "volumes", 1, 2)
			} else if len(op.columns) == 3 {
				rows.AddRow("p", "volumes", 1)
			} else {
				rows.AddRow("volumes", "1")
			}
			mock.ExpectQuery(regexp.QuoteMeta(op.query)).WillReturnRows(rows.RowError(0, failure)).RowsWillBeClosed()
			require.ErrorIs(t, op.read(r, context.Background()), failure)
			mock.ExpectQuery(regexp.QuoteMeta(op.query)).WillReturnRows(sqlmock.NewRows(op.columns).CloseError(failure)).RowsWillBeClosed()
			require.ErrorIs(t, op.read(r, context.Background()), failure)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestQuotaRepositoryScanFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		rows  *sqlmock.Rows
		read  func(*QuotaRepository) error
	}{
		{"limit_overflow", QuotaGetActiveLimits, sqlmock.NewRows([]string{"project_id", "resource", "hard_limit"}).AddRow("p", "volumes", int64(math.MaxInt32)+1), func(r *QuotaRepository) error { _, err := r.GetProjectLimits(context.Background()); return err }},
		{"usage_overflow", QuotaGetActiveUsage, sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).AddRow("p", "volumes", int64(math.MaxInt32)+1, 0), func(r *QuotaRepository) error { _, err := r.GetProjectUsage(context.Background()); return err }},
		{"reserved_underflow", QuotaGetActiveUsage, sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).AddRow("p", "volumes", 0, int64(math.MinInt32)-1), func(r *QuotaRepository) error { _, err := r.GetProjectUsage(context.Background()); return err }},
		{"usage_null", QuotaGetActiveUsage, sqlmock.NewRows([]string{"project_id", "resource", "in_use", "reserved"}).AddRow("p", "volumes", nil, 0), func(r *QuotaRepository) error { _, err := r.GetProjectUsage(context.Background()); return err }},
		{"default_overflow", QuotaGetDefaultClassLimits, sqlmock.NewRows([]string{"resource", "hard_limit"}).AddRow("volumes", int64(math.MaxInt32)+1), func(r *QuotaRepository) error { _, err := r.GetDefaultLimits(context.Background()); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery(regexp.QuoteMeta(tc.query)).WillReturnRows(tc.rows).RowsWillBeClosed()
			require.Error(t, tc.read(NewQuotaRepository(db)))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
