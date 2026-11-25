package oracleinfo_test

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
)

func TestTimeZoneFacadeReturnsActualNativeRows(t *testing.T) {
	step := testsql.Step{
		SQL:     "SELECT DBTIMEZONE AS DB_TIME_ZONE FROM SYS.DUAL",
		Columns: []string{"DB_TIME_ZONE"},
		Values:  [][]driver.Value{{"Europe/Paris"}},
	}
	db, _ := testsql.New(t, append(versionSteps("19"), step)...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).TimeZone(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows != spy.last || !rows.Next() {
		t.Fatalf("missing caller-native cursor: %v", rows.Err())
	}
	var value sql.NullString
	if err := rows.Scan(&value); err != nil || !value.Valid || value.String != "Europe/Paris" {
		t.Fatalf("altered region: %+v, %v", value, err)
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected scalar terminal result: %v", rows.Err())
	}
}

func TestSessionClockFacadeReturnsActualNativeRows(t *testing.T) {
	statement, err := database.SessionClockStatement(database.SessionClockOptions{OracleVersion: "19.0.0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{
		SQL:     statement,
		Columns: []string{"SYSDATE", "SYSTIMESTAMP_UTC", "CURRENT_DATE", "CURRENT_TIMESTAMP_UTC", "LOCALTIMESTAMP", "SESSION_TIME_ZONE"},
		Values:  [][]driver.Value{{"2026-11-25T12:00:00", "2026-11-25T12:00:00.012345678Z", "2026-11-25T09:00:00", "2026-11-25T12:00:00.012345000Z", "2026-11-25T09:00:00.012345000", "-03:00"}},
	}
	db, _ := testsql.New(t, append(versionSteps("19"), step)...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).SessionClock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows != spy.last || !rows.Next() {
		t.Fatalf("missing caller-native cursor: %v", rows.Err())
	}
	var civil, utc, currentDate, currentUTC, local, zone string
	if err = rows.Scan(&civil, &utc, &currentDate, &currentUTC, &local, &zone); err != nil ||
		utc != "2026-11-25T12:00:00.012345678Z" || local != "2026-11-25T09:00:00.012345000" || zone != "-03:00" {
		t.Fatalf("wrong clock values: %q %q %q, %v", utc, local, zone, err)
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected clock terminal result: %v", rows.Err())
	}
}
