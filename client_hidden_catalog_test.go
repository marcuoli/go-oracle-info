package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
)

func catalogStep() testsql.Step {
	return testsql.Step{Columns: []string{"NAME", "SESSION_VALUE", "INSTANCE_VALUE", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE", "DESCRIPTION"}, Values: [][]driver.Value{{"_waiters_test", "9007199254740993", nil, "TRUE", "DEFERRED", "description"}}}
}
func TestHiddenParameterCatalogBorrowsActualCursor(t *testing.T) {
	t.Parallel()
	db, state := testsql.New(t, append(versionSteps("23"), catalogStep())...)
	spy := &nativeSpy{db: db}
	rows, err := info.New(spy).HiddenParameterCatalog(t.Context(), info.HiddenCatalogFilter{NameContains: "WAITERS"})
	if err != nil || rows == nil || rows != spy.last {
		t.Fatal(rows, err)
	}
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	var name, value, instance, sessionMod, systemMod, description any
	if err = rows.Scan(&name, &value, &instance, &sessionMod, &systemMod, &description); err != nil {
		t.Fatal(err)
	}
	if name != "_waiters_test" || value != "9007199254740993" || instance != nil || sessionMod != "TRUE" || systemMod != "DEFERRED" {
		t.Fatal(name, value, instance, sessionMod, systemMod)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	sql, args, _ := parameters.HiddenCatalogStatement(23, parameters.HiddenCatalogFilter{NameContains: "WAITERS"})
	calls := state.Calls()
	last := calls[len(calls)-1]
	if last.SQL != sql || len(last.Args) != len(args) || last.Args[0].Value != "waiters" || state.Closed() != 3 {
		t.Fatal(last, state.Closed())
	}
}
func TestHiddenParameterCatalogJSONPreservesTextAndNull(t *testing.T) {
	t.Parallel()
	db, state := testsql.New(t, append(versionSteps("11"), catalogStep())...)
	data, err := info.New(db).HiddenParameterCatalogJSON(t.Context(), info.HiddenCatalogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Provenance parameters.Provenance
		Data       struct{ Rows [][]any }
	}
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	want := []any{"_waiters_test", "9007199254740993", nil, "TRUE", "DEFERRED", "description"}
	if len(decoded.Data.Rows) != 1 || !reflect.DeepEqual(decoded.Data.Rows[0], want) || decoded.Provenance.Scope != "connected_session_and_instance" || state.Closed() != 2 {
		t.Fatal(string(data))
	}
}
func TestHiddenParameterCatalogJSONPublishesNoFailedPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		configure func(*testsql.Step, context.CancelFunc)
		limits    info.JSONLimits
	}{
		{"terminal", func(s *testsql.Step, _ context.CancelFunc) { s.NextErr = errors.New("private terminal") }, info.JSONLimits{}},
		{"close", func(s *testsql.Step, _ context.CancelFunc) { s.CloseErr = errors.New("private close") }, info.JSONLimits{}},
		{"cancel", func(s *testsql.Step, cancel context.CancelFunc) { s.OnNext = func(int) { cancel() } }, info.JSONLimits{}},
		{"bytes", func(*testsql.Step, context.CancelFunc) {}, info.JSONLimits{MaxBytes: 1}},
		{"rows", func(s *testsql.Step, _ context.CancelFunc) { s.Values = append(s.Values, s.Values[0]) }, info.JSONLimits{MaxRows: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			step := catalogStep()
			tc.configure(&step, cancel)
			db, _ := testsql.New(t, append(versionSteps("23"), step)...)
			data, err := info.New(db).HiddenParameterCatalogJSON(ctx, info.HiddenCatalogFilter{}, tc.limits)
			if err == nil || data != nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("prefix or private data leaked: %s %v", data, err)
			}
		})
	}
}

func TestHiddenParameterCatalogInvalidInputsNeverQuery(t *testing.T) {
	t.Parallel()
	db, state := testsql.New(t)
	facts := info.New(db)
	if _, err := facts.HiddenParameterCatalog(t.Context(), info.HiddenCatalogFilter{Names: []string{"processes"}}); err == nil {
		t.Fatal("invalid filter admitted")
	}
	if _, err := facts.HiddenParameterCatalogJSON(t.Context(), info.HiddenCatalogFilter{}, info.JSONLimits{MaxBytes: -1}); err == nil {
		t.Fatal("invalid limits admitted")
	}
	if _, err := facts.HiddenParameterCatalogJSON(t.Context(), info.HiddenCatalogFilter{}, info.JSONLimits{}, info.JSONLimits{}); err == nil {
		t.Fatal("multiple budgets admitted")
	}
	if len(state.Calls()) != 0 {
		t.Fatal("invalid request queried", state.Calls())
	}
}
