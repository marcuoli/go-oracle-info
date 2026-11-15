package oracleinfo_test

import (
	"database/sql/driver"
	"encoding/json"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"testing"
)

func TestContextFacadeNativeIdentityAndVersionWiring(t *testing.T) {
	s := testsql.Step{Columns: []string{"INST_ID", "CONTEXT_ROW_INDEX", "PARAMETER_NUMBER", "VALUE_NUMBER", "NAME", "VALUE", "ISDEFAULT", "DESCRIPTION"}, Values: [][]driver.Value{{int64(1), int64(4), int64(10), int64(0), "compatible", "11.2.0.0.0", "FALSE", nil}}}
	db, state := testsql.New(t, append(versionSteps("11"), s)...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).HiddenParameterContext(t.Context(), info.HiddenContextFilter{Names: []string{"compatible"}})
	if err != nil || rows != spy.last || state.Closed() != 1 || !rows.Next() {
		t.Fatal(rows, err, state.Closed())
	}
	if err = rows.Close(); err != nil || state.Closed() != 2 {
		t.Fatal(err, state.Closed())
	}
	want, args, _ := parameters.HiddenContextStatement(11, parameters.HiddenContextFilter{Names: []string{"compatible"}})
	calls := state.Calls()
	last := calls[len(calls)-1]
	if last.SQL != want || len(last.Args) != len(args) || last.Args[0].Value != "compatible" {
		t.Fatal(last)
	}
	db, _ = testsql.New(t, append(versionSteps("11"), s)...)
	data, err := info.New(db).HiddenParameterContextJSON(t.Context(), info.HiddenContextFilter{})
	if err != nil || !json.Valid(data) {
		t.Fatal(string(data), err)
	}
}
func TestContextFacadeRejectsBeforeVersionQuery(t *testing.T) {
	db, state := testsql.New(t)
	c := info.New(db)
	if _, err := c.HiddenParameterContext(t.Context(), info.HiddenContextFilter{Names: []string{""}}); err == nil {
		t.Fatal("invalid scope accepted")
	}
	if _, err := c.HiddenParameterContextJSON(t.Context(), info.HiddenContextFilter{}, info.JSONLimits{MaxBytes: -1}); err == nil {
		t.Fatal("invalid budget accepted")
	}
	if len(state.Calls()) != 0 {
		t.Fatal(state.Calls())
	}
}
