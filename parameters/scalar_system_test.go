package parameters_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func scalarSystemFixture(display, basic, container bool) session.Result {
	source := systemResult(container)
	setValue(&source, 0, "VALUE", session.Value{Text: " 'one,two', 9007199254740993 "})
	setValue(&source, 0, "DISPLAY_VALUE", session.Value{Null: true})
	result := session.Result{Rows: [][]session.Value{{}}}
	for i, name := range source.Columns {
		switch name {
		case "DISPLAY_VALUE", "ISINSTANCE_MODIFIABLE", "ISDEPRECATED":
			if !display {
				continue
			}
		case "ISBASIC":
			if !basic {
				continue
			}
		}
		result.Columns = append(result.Columns, name)
		result.Rows[0] = append(result.Rows[0], source.Rows[0][i])
	}
	return result
}

func TestVersionedScalarSystemReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		version                   string
		major                     int
		display, basic, container bool
	}{
		{"9.2.0.8.0", 9, false, false, false},
		{"10.1.0.5.0", 10, true, false, false},
		{"10.2.0.5.0", 10, true, false, false},
		{"11.1.0.7.0", 11, true, true, false},
		{"11.2.0.4.0", 11, true, true, false},
		{"12.1.0.2.0", 12, true, true, true},
		{"12.2.0.1.0", 12, true, true, true},
		{"18.0.0.0.0", 18, true, true, true},
		{"19.32.0.0.0", 19, true, true, true},
		{"21.3.0.0.0", 21, true, true, true},
		{"23.2.0.0.0", 23, true, true, true},
		{"23.9.0.0.0", 23, true, true, true},
		{"23.26.1.0.0", 23, true, true, true},
		{"23.26.4.0.0", 23, true, true, true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := parameters.ScalarSystemOptions{
				OracleVersion: tc.version,
				Filter:        parameters.Filter{Names: []string{"_candidate"}, Instances: []int64{7}},
			}
			source := scalarSystemFixture(tc.display, tc.basic, tc.container)
			text, args, err := parameters.ScalarSystemStatement(options)
			aliases := make([]string, len(source.Columns))
			for i, name := range source.Columns {
				aliases[i] = "p." + name + " AS " + name
			}
			want := "SELECT " + strings.Join(aliases, ", ") + " FROM GV$SYSTEM_PARAMETER p WHERE "
			order := " ORDER BY p.INST_ID"
			if tc.container {
				want += "p.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))) AND "
				order += ", p.CON_ID"
			}
			want += "p.INST_ID IN (:1) AND NLSSORT(p.NAME, 'NLS_SORT=BINARY') IN (NLSSORT(:2, 'NLS_SORT=BINARY'))" + order + ", NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
			if err != nil || text != want || !reflect.DeepEqual(args, []any{int64(7), "_candidate"}) {
				t.Fatalf("statement = %q, %v, %v; want %q", text, args, err, want)
			}
			if tc.major >= 11 {
				existing, binds, err := parameters.SystemStatement(tc.major, options.Filter)
				if err != nil || existing != text || !reflect.DeepEqual(args, binds) {
					t.Fatalf("existing statement differs: %q, %v, %v", existing, binds, err)
				}
			}
			source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
			setValue(&source, 1, "VALUE", session.Value{Null: true})
			got, err := parameters.DecodeScalarSystem(source, options)
			if err != nil || got.State != parameters.ScalarSystemAvailable || got.Provenance.OracleVersion != tc.version ||
				got.Provenance.Complete || got.Provenance.OracleMajor != tc.major || got.Provenance.Source != "GV$SYSTEM_PARAMETER" || len(got.Rows) != 2 {
				t.Fatalf("unexpected system facts: %+v, %v", got, err)
			}
			row := got.Rows[0]
			if row.InstanceID.Text != "7" || row.Name.Text != "_candidate" || row.Value.Text != " 'one,two', 9007199254740993 " ||
				!got.Rows[1].Value.Null || (row.DisplayValue != nil) != tc.display || tc.display && !row.DisplayValue.Null ||
				(row.IsInstanceModifiable != nil) != tc.display || (row.IsDeprecated != nil) != tc.display ||
				(row.IsBasic != nil) != tc.basic || (row.ContainerID != nil) != tc.container || (row.IsPDBModifiable != nil) != tc.container {
				t.Fatalf("wrong raw scalar facts: %+v", got.Rows)
			}
			source.Rows = nil
			empty, err := parameters.DecodeScalarSystem(source, options)
			if err != nil || empty.State != parameters.ScalarSystemNoRows || empty.Rows == nil || len(empty.Rows) != 0 {
				t.Fatalf("empty = %+v, %v", empty, err)
			}
		})
	}
}

type scalarTestQueryer func(context.Context, string, ...any) (*sql.Rows, error)

func (f scalarTestQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return f(ctx, query, args...)
}

func TestScalarSystemDatabaseOnlyAndNativeOwnership(t *testing.T) {
	options := parameters.ScalarSystemOptions{OracleVersion: "23.26.4.0.0", Scope: parameters.ScalarSystemDatabaseOnly}
	text, args, err := parameters.ScalarSystemStatement(options)
	if err != nil || len(args) != 0 || !strings.Contains(text, "WHERE p.CON_ID = 0 ") || strings.Contains(text, "SYS_CONTEXT") {
		t.Fatalf("wrong scope query: %q, %v, %v", text, args, err)
	}
	source := scalarSystemFixture(true, true, true)
	setValue(&source, 0, "CON_ID", session.Value{Text: "0"})
	step := parameterNativeStep(t, source)
	step.SQL = text
	db, state := testsql.New(t, step, step)
	var native *sql.Rows
	rows, err := parameters.ScalarSystemRows(t.Context(), scalarTestQueryer(func(ctx context.Context, sql string, args ...any) (*sql.Rows, error) {
		rows, err := db.QueryContext(ctx, sql, args...)
		native = rows
		return rows, err
	}), options)
	if err != nil || rows == nil || rows != native || state.Closed() != 0 || !rows.Next() {
		t.Fatalf("caller cursor = %p, original = %p, err = %v", rows, native, err)
	}
	scanned := make([]any, len(source.Columns))
	dest := make([]any, len(scanned))
	for i := range dest {
		dest[i] = &scanned[i]
	}
	if err := rows.Scan(dest...); err != nil || scanned[0] != int64(7) || scanned[1] != int64(0) ||
		scanned[5] != " 'one,two', 9007199254740993 " || scanned[6] != nil {
		t.Fatalf("native row = %v, %v", scanned, err)
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected row or terminal error: %v", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := parameters.ScalarSystemJSON(t.Context(), db, options, rowset.Limits{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance parameters.ScalarSystemProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	columns := make([]string, len(envelope.Data.Columns))
	for i, column := range envelope.Data.Columns {
		columns[i] = column.Name
	}
	if envelope.Provenance.Scope != "whole_database_rows" || !envelope.Provenance.Complete || envelope.Provenance.OracleVersion != options.OracleVersion ||
		!reflect.DeepEqual(columns, source.Columns) || len(envelope.Data.Rows) != 1 ||
		envelope.Data.Rows[0][0] != float64(7) || envelope.Data.Rows[0][1] != float64(0) ||
		envelope.Data.Rows[0][5] != " 'one,two', 9007199254740993 " || envelope.Data.Rows[0][6] != nil || state.Closed() != 2 {
		t.Fatalf("incorrect scalar envelope: %s", payload)
	}

	source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
	setValue(&source, 1, "CON_ID", session.Value{Text: "3"})
	facts, err := parameters.DecodeScalarSystem(source, options)
	if facts.State != parameters.ScalarSystemFailed || facts.Rows != nil || !errors.Is(err, parameters.ErrInvalidEvidence) {
		t.Fatalf("other container published: %+v, %v", facts, err)
	}
	badDB, badState := testsql.New(t, parameterNativeStep(t, source))
	payload, err = parameters.ScalarSystemJSON(t.Context(), badDB, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, parameters.ErrInvalidEvidence) || badState.Closed() != 1 {
		t.Fatalf("other container published: %s, %v", payload, err)
	}
}

func TestScalarSystemReadFailureStatesAndLegacyJSON(t *testing.T) {
	options := parameters.ScalarSystemOptions{OracleVersion: "9.2.0.8.0", Scope: parameters.ScalarSystemDatabaseOnly}
	request := parameters.ScalarSystemRequest{Options: options, Reference: testReference(t)}
	source := scalarSystemFixture(false, false, false)
	text, _, _ := parameters.ScalarSystemStatement(options)
	calls := 0
	valid := parameterReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || s.Reference != request.Reference || s.Text != text || len(s.Args) != 0 || strings.Contains(s.Text, "CON_ID") {
			t.Fatalf("incorrect statement: %+v", s)
		}
		return source, nil
	})
	facts, err := parameters.ReadScalarSystem(t.Context(), valid, request)
	if err != nil || facts.State != parameters.ScalarSystemAvailable || !facts.Provenance.Complete || facts.Provenance.Scope != "non_cdb_visible_instance_rows" ||
		len(facts.Rows) != 1 || facts.Rows[0].DisplayValue != nil || calls != 1 {
		t.Fatalf("legacy buffered return = %+v, %v; calls = %d", facts, err, calls)
	}

	goodDB, _ := testsql.New(t, parameterNativeStep(t, source))
	payload, err := parameters.ScalarSystemJSON(t.Context(), goodDB, options, rowset.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance parameters.ScalarSystemProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.Complete || envelope.Provenance.OracleVersion != options.OracleVersion || len(envelope.Data.Columns) != len(source.Columns) ||
		len(envelope.Data.Rows) != 1 || envelope.Data.Rows[0][4] != " 'one,two', 9007199254740993 " ||
		strings.Contains(string(payload), "DISPLAY_VALUE") || strings.Contains(string(payload), "CON_ID") {
		t.Fatalf("legacy absence or raw value lost: %s", payload)
	}

	denied := errors.New("ORA-01031: private log text")
	unavailable := &session.Error{Code: "ORA-01219"}
	for _, tc := range []struct {
		cause, class error
		state        parameters.ScalarSystemState
	}{
		{denied, parameters.ErrSourceInaccessible, parameters.ScalarSystemInaccessible},
		{unavailable, parameters.ErrSourceUnavailable, parameters.ScalarSystemUnavailable},
		{errors.Join(denied, context.Canceled), nil, parameters.ScalarSystemFailed},
	} {
		r := parameterReader(func(context.Context, session.Statement) (session.Result, error) { return source, tc.cause })
		facts, err := parameters.ReadScalarSystem(t.Context(), r, request)
		if facts.State != tc.state || facts.Rows != nil || !errors.Is(err, tc.cause) ||
			tc.class != nil && !errors.Is(err, tc.class) || tc.class == nil && errors.Is(err, parameters.ErrSourceInaccessible) ||
			strings.Contains(err.Error(), "private") {
			t.Fatalf("incorrect failure: %+v, %v", facts, err)
		}
	}
	for _, native := range []bool{false, true} {
		db, state := testsql.New(t, testsql.Step{QueryErr: denied})
		if native {
			rows, err := parameters.ScalarSystemRows(t.Context(), db, options)
			if rows != nil || !errors.Is(err, parameters.ErrSourceInaccessible) || !errors.Is(err, denied) {
				t.Fatalf("native inaccessible source = %v, %v", rows, err)
			}
		} else {
			payload, err := parameters.ScalarSystemJSON(t.Context(), db, options, rowset.Limits{})
			if payload != nil || !errors.Is(err, parameters.ErrSourceInaccessible) || !errors.Is(err, denied) {
				t.Fatalf("JSON inaccessible source = %s, %v", payload, err)
			}
		}
		if len(state.Calls()) != 1 {
			t.Fatalf("source retried: %v", state.Calls())
		}
	}

	request.Options.OracleVersion = "26.0.0.0.0"
	facts, err = parameters.ReadScalarSystem(t.Context(), valid, request)
	if facts.State != parameters.ScalarSystemUnsupported || !errors.Is(err, parameters.ErrUnsupportedVersion) || calls != 1 {
		t.Fatalf("unsupported version = %+v, %v; calls = %d", facts, err, calls)
	}
	request.Options = options
	request.Options.Scope = 9
	facts, err = parameters.ReadScalarSystem(t.Context(), valid, request)
	if facts.State != parameters.ScalarSystemFailed || !errors.Is(err, parameters.ErrInvalidScalarSystemScope) || calls != 1 {
		t.Fatalf("invalid scope = %+v, %v; calls = %d", facts, err, calls)
	}
}
