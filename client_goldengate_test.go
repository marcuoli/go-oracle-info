package oracleinfo_test

import (
	"database/sql"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"testing"
)

func TestGoldenGateFacadeReturnsActualNativeCursor(t *testing.T) {
	for _, apply := range []bool{false, true} {
		statement, e := goldengate.CaptureStatement("19.26.1.0.0")
		columns := []string{"INST_ID", "CAPTURE_NAME", "STATE", "CAPTURE_MESSAGE_NUMBER", "TOTAL_MESSAGES_CAPTURED", "CON_ID"}
		if apply {
			statement, e = goldengate.ApplyCoordinatorsStatement("19.26.1.0.0")
			columns = []string{"INST_ID", "APPLY_NAME", "STATE", "TOTAL_APPLIED", "TOTAL_ERRORS", "LWM_MESSAGE_NUMBER", "CON_ID"}
		}
		if e != nil {
			t.Fatal(e)
		}
		step := testsql.Step{SQL: statement, Columns: columns}
		db, state := testsql.New(t, append(versionSteps("19"), step)...)
		spy := &nativeSpy{db: db}
		var rows *sql.Rows
		var err error
		if apply {
			rows, err = info.New(spy).GoldenGateApplyCoordinators(t.Context())
		} else {
			rows, err = info.New(spy).GoldenGateCapture(t.Context())
		}
		if err != nil || rows != spy.last {
			t.Fatal("cursor identity", err)
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
		if state.Closed() != 3 {
			t.Fatal("lifecycle", state.Closed())
		}
		db, state = testsql.New(t, append(versionSteps("19"), step)...)
		var data []byte
		if apply {
			data, err = info.New(db).GoldenGateApplyCoordinatorsJSON(t.Context())
		} else {
			data, err = info.New(db).GoldenGateCaptureJSON(t.Context())
		}
		if err != nil || len(data) == 0 || state.Closed() != 3 {
			t.Fatal(string(data), err)
		}
	}
}
