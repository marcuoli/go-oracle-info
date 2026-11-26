package parameters_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func extendedHiddenCatalogResult() session.Result {
	result := hiddenCatalogResult()
	result.Columns = append(result.Columns, "SESSION_IS_DEFAULT")
	result.Rows[0] = append(result.Rows[0], session.Value{Text: " future flag "})
	result.Rows[1] = append(result.Rows[1], session.Value{Null: true})
	return result
}

func TestHiddenCatalogWithSessionDefaultSelection(t *testing.T) {
	t.Parallel()
	filter := parameters.HiddenCatalogFilter{Names: []string{"_waiters_test"}, NameContains: "WAITERS"}
	const column = ", b.KSPPSTDF AS SESSION_IS_DEFAULT"
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23} {
		legacy, legacyArgs, legacyErr := parameters.HiddenCatalogStatement(major, filter)
		stmt, args, err := parameters.HiddenCatalogWithSessionDefaultStatement(major, filter)
		if major < 11 {
			if !errors.Is(err, parameters.ErrUnsupportedVersion) || stmt != "" || args != nil {
				t.Fatalf("major %d submitted unsupported SQL: %q %v", major, stmt, err)
			}
			continue
		}
		if legacyErr != nil || err != nil || !slices.Equal(legacyArgs, args) || strings.Count(stmt, column) != 1 ||
			strings.Replace(stmt, column, "", 1) != legacy || strings.Contains(legacy, "KSPPSTDF") {
			t.Fatalf("major %d did not select only the extra flag: %q %v %v", major, stmt, args, err)
		}
	}
}

func TestReadHiddenCatalogWithSessionDefaultPreservesRawFlag(t *testing.T) {
	t.Parallel()
	request := parameters.HiddenCatalogRequest{
		OracleMajor: 23,
		Filter:      parameters.HiddenCatalogFilter{NameContains: "WAITERS"},
		Reference:   testReference(t),
	}
	calls := 0
	reader := parameterReader(func(_ context.Context, statement session.Statement) (session.Result, error) {
		calls++
		if !strings.Contains(statement.Text, "b.KSPPSTDF AS SESSION_IS_DEFAULT") || !slices.Equal(statement.Args, []any{"waiters"}) {
			t.Fatalf("incorrect projection or binds: %q %v", statement.Text, statement.Args)
		}
		return extendedHiddenCatalogResult(), nil
	})
	legacy, err := parameters.DecodeHiddenCatalog(hiddenCatalogResult(), 23, request.Filter)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parameters.ReadHiddenCatalogWithSessionDefault(t.Context(), reader, request)
	if err != nil || calls != 1 || len(got.Rows) != 2 {
		t.Fatalf("read: %v %v %d", got, err, calls)
	}
	if got.Rows[0].HiddenParameter != legacy.Rows[0] || got.Rows[1].HiddenParameter != legacy.Rows[1] ||
		got.Rows[0].SessionIsDefault != (session.Value{Text: " future flag "}) || !got.Rows[1].SessionIsDefault.Null ||
		got.Provenance.Source != legacy.Provenance.Source || got.Provenance.Scope != "connected_session_and_instance" {
		t.Fatalf("catalog data changed or default was coerced: %+v", got)
	}

	incomplete := extendedHiddenCatalogResult()
	incomplete.Rows[1][6] = session.Value{Text: "contradictory", Null: true}
	badReader := parameterReader(func(context.Context, session.Statement) (session.Result, error) { return incomplete, nil })
	got, err = parameters.ReadHiddenCatalogWithSessionDefault(t.Context(), badReader, request)
	if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
		t.Fatalf("published malformed late result: %+v %v", got, err)
	}
	if _, err = parameters.DecodeHiddenCatalogWithSessionDefault(hiddenCatalogResult(), 23, request.Filter); !errors.Is(err, parameters.ErrInvalidEvidence) {
		t.Fatalf("accepted absent flag: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelingReader := parameterReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return extendedHiddenCatalogResult(), nil
	})
	got, err = parameters.ReadHiddenCatalogWithSessionDefault(ctx, cancelingReader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatalf("published canceled result: %+v %v", got, err)
	}
}

func TestHiddenCatalogWithSessionDefaultNativeAndJSONShape(t *testing.T) {
	t.Parallel()
	filter := parameters.HiddenCatalogFilter{}
	step := testsql.Step{
		Columns: extendedHiddenCatalogResult().Columns,
		Values: [][]driver.Value{
			{"_one", "session", "instance", "TRUE", "FALSE", "description", "literal NULL"},
			{"_two", nil, "instance", "FALSE", "IMMEDIATE", nil, nil},
		},
	}
	db, state := testsql.New(t, step)
	rows, err := parameters.HiddenCatalogWithSessionDefaultRows(t.Context(), db, 23, filter)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := rows.Columns()
	if err != nil || !slices.Equal(columns, step.Columns) || state.Closed() != 0 || !rows.Next() {
		t.Fatalf("unexpected native cursor %v %v", columns, err)
	}
	if err = rows.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}

	db, _ = testsql.New(t, step)
	data, err := parameters.HiddenCatalogWithSessionDefaultJSON(t.Context(), db, 23, filter, rowset.Limits{})
	var envelope struct {
		Data struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err != nil || json.Unmarshal(data, &envelope) != nil || len(envelope.Data.Columns) != len(step.Columns) ||
		len(envelope.Data.Rows) != 2 || len(envelope.Data.Rows[0]) != 7 || len(envelope.Data.Rows[1]) != 7 ||
		envelope.Data.Rows[0][6] != "literal NULL" || envelope.Data.Rows[1][6] != nil {
		t.Fatalf("invalid JSON flag shape: %s %v", data, err)
	}

	for i, column := range envelope.Data.Columns {
		if column.Name != step.Columns[i] || column.Kind != rowset.Text {
			t.Fatalf("column %d: %+v", i, column)
		}
	}

	db, state = testsql.New(t, step)
	data, err = parameters.HiddenCatalogWithSessionDefaultJSON(t.Context(), db, 23, filter, rowset.Limits{MaxRows: 1})
	if !errors.Is(err, rowset.ErrCapacity) || data != nil || state.Closed() != 1 {
		t.Fatalf("published over-budget result: %s %v", data, err)
	}
}
