package neutron

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

var reservationColumns = []string{"reservation_id", "project_id", "expiration", "resource", "amount"}

func reservationQuery(placeholders string) string {
	return regexp.QuoteMeta(strings.Replace(GetReservationsForResources, "/*SLICE:resources*/?", placeholders, 1))
}

func TestGetReservationsForResourcesScansRawRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { mock.ExpectClose(); require.NoError(t, db.Close()) })

	expiration := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(reservationQuery("?,?")).
		WithArgs("port", "network").
		WillReturnRows(sqlmock.NewRows(reservationColumns).
			AddRow("r1", "p", expiration, "port", 3).
			AddRow("r2", "", expiration, "network", -2).
			AddRow("r3", "p", expiration, "port", nil)).
		RowsWillBeClosed()

	rows, err := New(db).GetReservationsForResources(context.Background(), []string{"port", "network"})
	require.NoError(t, err)
	require.Equal(t, []GetReservationsForResourcesRow{
		{ReservationID: "r1", ProjectID: sql.NullString{String: "p", Valid: true}, Expiration: sql.NullTime{Time: expiration, Valid: true}, Resource: "port", Amount: sql.NullInt32{Int32: 3, Valid: true}},
		{ReservationID: "r2", ProjectID: sql.NullString{Valid: true}, Expiration: sql.NullTime{Time: expiration, Valid: true}, Resource: "network", Amount: sql.NullInt32{Int32: -2, Valid: true}},
		{ReservationID: "r3", ProjectID: sql.NullString{String: "p", Valid: true}, Expiration: sql.NullTime{Time: expiration, Valid: true}, Resource: "port"},
	}, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

// An empty resource list matches nothing, like upstream Neutron's
// get_reservations_for_resources, instead of widening to every resource.
func TestGetReservationsForResourcesEmptyListMatchesNothing(t *testing.T) {
	for name, resources := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { mock.ExpectClose(); require.NoError(t, db.Close()) })
			mock.ExpectQuery(reservationQuery("NULL")).WithoutArgs().
				WillReturnRows(sqlmock.NewRows(reservationColumns)).RowsWillBeClosed()

			rows, err := New(db).GetReservationsForResources(context.Background(), resources)
			require.NoError(t, err)
			require.Empty(t, rows)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGetReservationsForResourcesPropagatesErrors(t *testing.T) {
	for _, kind := range []string{"query", "scan", "iteration", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { mock.ExpectClose(); require.NoError(t, db.Close()) })

			exp := mock.ExpectQuery(reservationQuery("?")).WithArgs("port")
			rows := sqlmock.NewRows(reservationColumns)
			switch kind {
			case "query":
				exp.WillReturnError(errors.New("read failure"))
			case "scan":
				exp.WillReturnRows(rows.AddRow("r", "p", time.Now(), "port", "malformed")).RowsWillBeClosed()
			case "iteration":
				exp.WillReturnRows(rows.AddRow("r", "p", time.Now(), "port", 1).RowError(0, errors.New("interrupted"))).RowsWillBeClosed()
			case "timeout":
				exp.WillDelayFor(time.Second).WillReturnRows(rows)
			}
			timeout := time.Second
			if kind == "timeout" {
				timeout = 10 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			got, err := New(db).GetReservationsForResources(ctx, []string{"port"})
			require.Error(t, err)
			require.Nil(t, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
