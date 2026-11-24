package oracleinfo_test

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
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
