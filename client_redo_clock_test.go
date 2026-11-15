package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/redo"
)

func nativeClockStep(major int) testsql.Step {
	statement, _ := redo.ClockStatement(major)
	step := testsql.Step{SQL: statement, Columns: []string{"INST_ID", "LOCAL_TIME"}, Values: [][]driver.Value{{"1", "2026-11-15T23:59:58"}}}
	if major >= 12 {
		step.Columns = append(step.Columns, "CON_ID")
		step.Values[0] = append(step.Values[0], nil)
	}
	return step
}
func TestRedoClockFacadeBorrowsActualCursor(t *testing.T) {
	db, _ := testsql.New(t, append(versionSteps("19"), nativeClockStep(19))...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).RedoClock(t.Context())
	if err != nil || rows != spy.last {
		t.Fatalf("native clock identity: %v %v", rows, err)
	}
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestRedoClockFacadeJSONVersionAndBudget(t *testing.T) {
	db, _ := testsql.New(t, append(versionSteps("11"), nativeClockStep(11))...)
	payload, err := info.New(db).RedoClockJSON(t.Context(), info.JSONLimits{MaxRows: 1, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data struct{ Rows [][]any } }
	if err = json.Unmarshal(payload, &envelope); err != nil || len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 2 || envelope.Data.Rows[0][1] != "2026-11-15T23:59:58" {
		t.Fatalf("facade lost source label/11g projection: %s %v", payload, err)
	}
	empty, state := testsql.New(t)
	if _, err = info.New(empty).RedoClockJSON(t.Context(), info.JSONLimits{MaxRows: -1}); err == nil || len(state.Calls()) != 0 {
		t.Fatal("invalid clock budget queried source/version")
	}
}
