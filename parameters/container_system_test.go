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

func TestContainerSystemSelection(t *testing.T) {
	for _, version := range []string{"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.0.0.0.0", "19.32.0.0.0", "21.0.0.0.0", "23.4.0.0.0", "23.6.0.0.0", "23.26.4.0.0"} {
		t.Run(version, func(t *testing.T) {
			for _, tc := range []struct {
				source parameters.ContainerSystemSource
				inst   string
				view   string
			}{
				{parameters.ContainerSystemLocal, "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE'))", "V$SYSTEM_PARAMETER"},
				{parameters.ContainerSystemGlobal, "p.INST_ID", "GV$SYSTEM_PARAMETER"},
			} {
				options := parameters.ContainerSystemOptions{OracleVersion: version, CurrentContainerID: 1, Source: tc.source,
					Filter:       parameters.Filter{Names: []string{"cpu_count", "sga_target"}, Instances: []int64{7}},
					ContainerIDs: []int64{0, 3}}
				sqlText, args, err := parameters.ContainerSystemStatement(options)
				want := "SELECT " + tc.inst + " AS INST_ID, p.CON_ID AS CON_ID, p.NAME AS NAME, p.VALUE AS VALUE, p.DISPLAY_VALUE AS DISPLAY_VALUE FROM " + tc.view + " p WHERE p.CON_ID IN (:1,:2) AND " + tc.inst + " IN (:3) AND NLSSORT(p.NAME, 'NLS_SORT=BINARY') IN (NLSSORT(:4, 'NLS_SORT=BINARY'),NLSSORT(:5, 'NLS_SORT=BINARY')) ORDER BY INST_ID, p.CON_ID, NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
				if err != nil || sqlText != want || !reflect.DeepEqual(args, []any{int64(0), int64(3), int64(7), "cpu_count", "sga_target"}) {
					t.Fatalf("scoped source mismatch: %q %#v %v", sqlText, args, err)
				}
			}
		})
	}
	for _, version := range []string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.3.0.0.0", "22.1.0.0.0", "26.4.0.0.0", "23", "+23.4", "23ai", "23.26.4.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			options := parameters.ContainerSystemOptions{OracleVersion: version, CurrentContainerID: 1, Source: parameters.ContainerSystemGlobal}
			sqlText, args, err := parameters.ContainerSystemStatement(options)
			if sqlText != "" || args != nil || !errors.Is(err, parameters.ErrUnsupportedVersion) {
				t.Fatalf("unsupported release selected SQL: %q %#v %v", sqlText, args, err)
			}
			db, state := testsql.New(t)
			rows, err := parameters.ContainerSystemRows(t.Context(), db, options)
			if rows != nil || !errors.Is(err, parameters.ErrUnsupportedVersion) || len(state.Calls()) != 0 {
				t.Fatalf("unsupported release opened cursor: %v %v", rows, err)
			}
		})
	}
}

func TestContainerSystemScope(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "23.26.4.0.0", CurrentContainerID: 1, Source: parameters.ContainerSystemGlobal}
	sqlText, args, err := parameters.ContainerSystemStatement(options)
	if err != nil || len(args) != 0 || strings.Contains(sqlText, " WHERE ") {
		t.Fatal(sqlText, args, err)
	}
	options.CurrentContainerID = 3
	sqlText, args, err = parameters.ContainerSystemStatement(options)
	if err != nil || !reflect.DeepEqual(args, []any{int64(3)}) || !strings.Contains(sqlText, "WHERE p.CON_ID IN (:1) ORDER BY") {
		t.Fatal(sqlText, args, err)
	}
	options.ContainerIDs = []int64{0, 3}
	if _, _, err := parameters.ContainerSystemStatement(options); !errors.Is(err, parameters.ErrInvalidFilter) {
		t.Fatal(err)
	}
	options.ContainerIDs, options.CurrentContainerID = nil, 0
	if _, _, err := parameters.ContainerSystemStatement(options); !errors.Is(err, parameters.ErrContainerSystemUnsupportedScope) {
		t.Fatal(err)
	}
	options.CurrentContainerID, options.Source = 1, "DUAL"
	if sqlText, _, err := parameters.ContainerSystemStatement(options); sqlText != "" || !errors.Is(err, parameters.ErrInvalidFilter) {
		t.Fatal(sqlText, err)
	}
}

func containerSystemFixture() session.Result {
	return session.Result{
		Columns: []string{"INST_ID", "CON_ID", "NAME", "VALUE", "DISPLAY_VALUE"},
		Rows: [][]session.Value{
			{{Text: "7"}, {Text: "3"}, {Text: "cpu_count"}, {Text: "0"}, {Null: true}},
			{{Text: "7"}, {Text: "3"}, {Text: "cpu_count"}, {Null: true}, {Text: " formatted "}},
			{{Text: "8"}, {Text: "9"}, {Text: "sga_target"}, {Text: "9007199254740993123456789"}, {Text: "28G"}},
		},
	}
}

func TestContainerSystemReadAndDecode(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "23.26.4.0.0", CurrentContainerID: 1, Source: parameters.ContainerSystemGlobal,
		ContainerIDs: []int64{3, 9}, Filter: parameters.Filter{Names: []string{"cpu_count", "sga_target"}}}
	ref := testReference(t)
	request := parameters.ContainerSystemRequest{Options: options, Reference: ref}
	source := containerSystemFixture()
	sqlText, args, err := parameters.ContainerSystemStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	reader := parameterReader(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
		calls++
		if stmt.Reference != ref || stmt.Text != sqlText || !reflect.DeepEqual(stmt.Args, args) {
			t.Fatalf("read changed selection: %#v", stmt)
		}
		return source, nil
	})
	result, err := parameters.ReadContainerSystem(t.Context(), reader, request)
	wantRows := []parameters.ContainerSystemParameter{
		{InstanceID: session.Value{Text: "7"}, ContainerID: session.Value{Text: "3"}, Name: session.Value{Text: "cpu_count"}, Value: session.Value{Text: "0"}, DisplayValue: session.Value{Null: true}},
		{InstanceID: session.Value{Text: "7"}, ContainerID: session.Value{Text: "3"}, Name: session.Value{Text: "cpu_count"}, Value: session.Value{Null: true}, DisplayValue: session.Value{Text: " formatted "}},
		{InstanceID: session.Value{Text: "8"}, ContainerID: session.Value{Text: "9"}, Name: session.Value{Text: "sga_target"}, Value: session.Value{Text: "9007199254740993123456789"}, DisplayValue: session.Value{Text: "28G"}},
	}
	if err != nil || calls != 1 || !reflect.DeepEqual(result.Rows, wantRows) || result.Provenance.OracleVersion != options.OracleVersion ||
		result.Provenance.Source != "GV$SYSTEM_PARAMETER" || result.Provenance.CurrentContainerID != 1 ||
		!reflect.DeepEqual(result.Provenance.ContainerIDs, []int64{3, 9}) {
		t.Fatal(result, err, calls)
	}
	options.ContainerIDs[0] = 4
	options.Filter.Names[0] = "renamed"
	if result.Provenance.ContainerIDs[0] != 3 || result.Provenance.Names[0] != "cpu_count" {
		t.Fatal("provenance aliases input filters", result.Provenance)
	}
	options.ContainerIDs, options.Filter.Names = []int64{3, 9}, []string{"cpu_count", "sga_target"}
	source.Rows = nil
	empty, err := parameters.DecodeContainerSystem(source, options)
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatal(empty, err)
	}
	source = containerSystemFixture()
	options.CurrentContainerID, options.ContainerIDs = 3, nil
	if result, err := parameters.DecodeContainerSystem(source, options); result.Rows != nil || !errors.Is(err, parameters.ErrInvalidEvidence) {
		t.Fatal(result, err)
	}
	source = containerSystemFixture()
	source.Columns = source.Columns[:4]
	if result, err := parameters.DecodeContainerSystem(source, options); result.Rows != nil || !errors.Is(err, parameters.ErrInvalidEvidence) {
		t.Fatal(result, err)
	}
}

type containerSystemQueryFunc func(context.Context, string, ...any) (*sql.Rows, error)

func (f containerSystemQueryFunc) QueryContext(ctx context.Context, stmt string, args ...any) (*sql.Rows, error) {
	return f(ctx, stmt, args...)
}

func TestContainerSystemNativeRowsAndJSON(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "12.2.0.1.0", CurrentContainerID: 3, Source: parameters.ContainerSystemLocal,
		Filter: parameters.Filter{Names: []string{"cpu_count"}, Instances: []int64{7}}}
	source := containerSystemFixture()
	source.Rows = source.Rows[:2]
	step := parameterNativeStep(t, source)
	step.SQL, _, _ = parameters.ContainerSystemStatement(options)
	db, state := testsql.New(t, step, step)
	var actual *sql.Rows
	q := containerSystemQueryFunc(func(ctx context.Context, stmt string, args ...any) (*sql.Rows, error) {
		var err error
		actual, err = db.QueryContext(ctx, stmt, args...)
		return actual, err
	})
	rows, err := parameters.ContainerSystemRows(t.Context(), q, options)
	if err != nil || rows != actual || rows == nil || !rows.Next() {
		t.Fatalf("native cursor identity/use: %p %p %v", rows, actual, err)
	}
	var id, conID int64
	var name, value string
	var display sql.NullString
	if err := rows.Scan(&id, &conID, &name, &value, &display); err != nil || id != 7 || conID != 3 || name != "cpu_count" || value != "0" || display.Valid {
		t.Fatalf("native value mismatch: %v %v %q %q %#v %v", id, conID, name, value, display, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := parameters.ContainerSystemJSON(t.Context(), db, options, rowset.Limits{MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Provenance parameters.ContainerSystemProvenance
		Data       rowset.Table
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	wantColumns := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CON_ID", Kind: rowset.Integer},
		{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}, {Name: "DISPLAY_VALUE", Kind: rowset.Text},
	}
	wantRows := [][]any{{float64(7), float64(3), "cpu_count", "0", nil}, {float64(7), float64(3), "cpu_count", nil, " formatted "}}
	if decoded.Provenance.Source != "V$SYSTEM_PARAMETER" || decoded.Provenance.CurrentContainerID != 3 ||
		!reflect.DeepEqual(decoded.Provenance.ContainerIDs, []int64{3}) ||
		!reflect.DeepEqual(decoded.Data.Columns, wantColumns) || !reflect.DeepEqual(decoded.Data.Rows, wantRows) ||
		state.Closed() != 2 || len(state.Calls()) != 2 {
		t.Fatal(decoded, state.Closed(), state.Calls())
	}
	for _, call := range state.Calls() {
		if len(call.Args) != 3 || call.Args[0].Value != int64(3) || call.Args[1].Value != int64(7) || call.Args[2].Value != "cpu_count" {
			t.Fatal(call)
		}
	}
}

func TestContainerSystemErrorsNeverPublishPrefix(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "19.32.0.0.0", CurrentContainerID: 3, Source: parameters.ContainerSystemLocal}
	source := containerSystemFixture()
	source.Rows = source.Rows[:2]
	privateFailure := errors.New("private cause")
	for _, tc := range []struct {
		name   string
		change func(*testsql.Step)
		limits rowset.Limits
		want   error
	}{
		{"shape", func(s *testsql.Step) { s.Columns = []string{"ID", "CON_ID", "NAME", "VALUE", "DISPLAY_VALUE"} }, rowset.Limits{}, rowset.ErrInvalidEvidence},
		{"late container", func(s *testsql.Step) { s.Values[1][1] = int64(4) }, rowset.Limits{}, parameters.ErrInvalidEvidence},
		{"next", func(s *testsql.Step) { s.NextErr = privateFailure }, rowset.Limits{}, privateFailure},
		{"close", func(s *testsql.Step) { s.CloseErr = privateFailure }, rowset.Limits{}, privateFailure},
		{"row limit", func(*testsql.Step) {}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
		{"byte limit", func(*testsql.Step) {}, rowset.Limits{MaxBytes: 1}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			step := parameterNativeStep(t, source)
			tc.change(&step)
			db, _ := testsql.New(t, step)
			if data, err := parameters.ContainerSystemJSON(t.Context(), db, options, tc.limits); data != nil || !errors.Is(err, tc.want) {
				t.Fatalf("failure returned partial data: %s %v", data, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	result, err := parameters.ReadContainerSystem(ctx, parameterReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return source, nil
	}), parameters.ContainerSystemRequest{Options: options, Reference: testReference(t)})
	if result.Rows != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(result, err)
	}
}

func TestContainerSystemSourceDiagnostics(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "19.32.0.0.0", CurrentContainerID: 1, Source: parameters.ContainerSystemGlobal}
	for _, tc := range []struct {
		name    string
		failure error
		denied  bool
	}{
		{"explicit plain", errors.New("ORA-01031: private denied"), true},
		{"explicit unpadded go-ora", errors.New("ORA-1031 error occur at position: 2"), true},
		{"nonspecific message", errors.New("ORA-00942: query mentions ORA-01031"), false},
		{"explicit session", &session.Error{Code: "ORA-01031"}, true},
		{"ambiguous", errors.New("ORA-00942: private missing"), false},
		{"other", errors.New("private transport"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check := func(err error) {
				t.Helper()
				var failure *parameters.Error
				if !errors.As(err, &failure) || failure.Source != "GV$SYSTEM_PARAMETER" ||
					!errors.Is(err, tc.failure) || strings.Contains(err.Error(), "private") ||
					errors.Is(err, parameters.ErrContainerSystemInaccessible) != tc.denied || errors.Is(err, parameters.ErrUnsupportedVersion) {
					t.Fatalf("typed, sanitized diagnostics lost: %v", err)
				}
			}
			req := parameters.ContainerSystemRequest{Options: options, Reference: testReference(t)}
			got, err := parameters.ReadContainerSystem(t.Context(), parameterReader(func(context.Context, session.Statement) (session.Result, error) {
				return containerSystemFixture(), tc.failure
			}), req)
			if got.Rows != nil {
				t.Fatal(got)
			}
			check(err)
			db, _ := testsql.New(t, testsql.Step{QueryErr: tc.failure}, testsql.Step{QueryErr: tc.failure})
			rows, err := parameters.ContainerSystemRows(t.Context(), db, options)
			if rows != nil {
				t.Fatal(rows)
			}
			check(err)
			data, err := parameters.ContainerSystemJSON(t.Context(), db, options, rowset.Limits{})
			if data != nil {
				t.Fatal(data)
			}
			check(err)
		})
	}
}
