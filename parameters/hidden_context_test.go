package parameters_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func contextResult(modern bool) session.Result {
	r := session.Result{Columns: []string{"INST_ID", "CONTEXT_ROW_INDEX", "PARAMETER_NUMBER", "VALUE_NUMBER", "NAME", "VALUE", "ISDEFAULT", "DESCRIPTION"}, Rows: [][]session.Value{{{Text: "7"}, {Text: "9007199254740993"}, {Text: "42"}, {Text: "0"}, {Text: "compatible"}, {Text: "19.0.0"}, {Text: "FALSE"}, {Text: "raw description"}}, {{Text: "7"}, {Text: "9"}, {Text: "43"}, {Text: "1"}, {Text: "_example"}, {Null: true}, {Text: "future"}, {Null: true}}, {{Text: "7"}, {Text: "10"}, {Text: "43"}, {Text: "2"}, {Text: "_example"}, {Text: "second\nvalue"}, {Null: true}, {Text: "different context"}}}}
	if modern {
		r.Columns = append(r.Columns[:1], append([]string{"CON_ID"}, r.Columns[1:]...)...)
		for i, row := range r.Rows {
			r.Rows[i] = append(row[:1], append([]session.Value{{Null: true}}, row[1:]...)...)
		}
	}
	return r
}
func contextStep(modern bool) testsql.Step {
	r := contextResult(modern)
	values := make([][]driver.Value, len(r.Rows))
	for i, row := range r.Rows {
		values[i] = make([]driver.Value, len(row))
		for j, v := range row {
			if !v.Null {
				values[i][j] = v.Text
			}
		}
	}
	return testsql.Step{Columns: r.Columns, Values: values}
}
func TestContextSourcePreservesOffsetOrdinaryNamesAndRawFlags(t *testing.T) {
	for _, major := range []int{11, 19, 23, 26} {
		sql, args, err := parameters.HiddenContextStatement(major, parameters.HiddenContextFilter{Names: []string{"compatible", "_example"}})
		if err != nil || !strings.Contains(sql, "JOIN SYS.X$KSPPCV2 b ON a.INDX + 1 = b.KSPFTCTXPN") || strings.Contains(sql, "SUBSTR(") || strings.Contains(sql, "KSPPSTVL") || len(args) != 2 || args[0] != "compatible" || strings.Contains(sql, "'compatible'") || (strings.Contains(sql, "b.CON_ID AS CON_ID") != (major >= 12)) {
			t.Fatal(sql, args, err)
		}
		result := contextResult(major >= 12)
		got, err := parameters.DecodeHiddenContext(result, major, parameters.HiddenContextFilter{})
		if err != nil || len(got.Rows) != 3 || got.Rows[0].Name.Text != "compatible" || got.Rows[0].ContextRowIndex.Text != "9007199254740993" || got.Rows[1].ParameterNumber.Text != "43" || got.Rows[2].ValueNumber.Text != "2" || !got.Rows[1].Value.Null || !got.Rows[2].IsDefault.Null || got.Rows[1].IsDefault.Text != "future" || got.Provenance.Source != "SYS.X$KSPPI+SYS.X$KSPPCV2" || got.Provenance.Scope != "connected_instance_context_rows" {
			t.Fatal(got, err)
		}
		if (got.Rows[0].ContainerID != nil) != (major >= 12) || (got.Rows[0].ContainerID != nil && !got.Rows[0].ContainerID.Null) {
			t.Fatal("absent/projected NULL conflated", got)
		}
	}
	if _, _, err := parameters.HiddenContextStatement(10, parameters.HiddenContextFilter{}); !errors.Is(err, parameters.ErrUnsupportedVersion) {
		t.Fatal(err)
	}
}
func TestContextRawDefaultAndExactNameAdmissionMatchesSQL(t *testing.T) {
	filter := parameters.HiddenContextFilter{NonDefaultOnly: true}
	sql, _, err := parameters.HiddenContextStatement(19, filter)
	if err != nil || !strings.Contains(sql, "NLSSORT(b.KSPFTCTXDF, 'NLS_SORT=BINARY') <> NLSSORT('TRUE', 'NLS_SORT=BINARY')") {
		t.Fatal(sql, err)
	}
	for _, flag := range []session.Value{{Text: "TRUE"}, {Null: true}, {Text: "FALSE"}, {Text: "true"}, {Text: "future"}} {
		r := contextResult(false)
		r.Rows = r.Rows[:1]
		r.Rows[0][6] = flag
		got, err := parameters.DecodeHiddenContext(r, 11, filter)
		wantBad := flag.Null || flag.Text == "TRUE"
		if wantBad {
			if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
				t.Fatal(flag, got, err)
			}
		} else if err != nil || got.Rows[0].IsDefault != flag {
			t.Fatal(flag, got, err)
		}
	}
	r := contextResult(false)
	r.Rows = r.Rows[:1]
	if _, err = parameters.DecodeHiddenContext(r, 11, parameters.HiddenContextFilter{Names: []string{"COMPATIBLE"}}); !errors.Is(err, parameters.ErrInvalidEvidence) {
		t.Fatal("case scope mismatch accepted", err)
	}
	r.Rows = nil
	got, err := parameters.DecodeHiddenContext(r, 11, parameters.HiddenContextFilter{})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatal(got, err)
	}
}
func TestContextRejectsEveryMalformedLateRowWithoutPrefix(t *testing.T) {
	for _, change := range []func(*session.Result){func(r *session.Result) { r.Columns = r.Columns[1:] }, func(r *session.Result) { r.Columns[0] = "NAME" }, func(r *session.Result) { r.Rows[2] = r.Rows[2][:1] }, func(r *session.Result) { r.Rows[2][0] = session.Value{Null: true} }, func(r *session.Result) { r.Rows[2][1].Text = "1.5" }, func(r *session.Result) { r.Rows[2][2].Text = "0" }, func(r *session.Result) { r.Rows[2][3].Text = "9223372036854775808" }, func(r *session.Result) { r.Rows[2][4] = session.Value{Null: true} }, func(r *session.Result) { r.Rows[2][5] = session.Value{Text: string([]byte{255})} }, func(r *session.Result) { r.Rows[2][6] = session.Value{Text: "private", Null: true} }} {
		r := contextResult(false)
		change(&r)
		got, err := parameters.DecodeHiddenContext(r, 11, parameters.HiddenContextFilter{})
		if !errors.Is(err, parameters.ErrInvalidEvidence) || got.Rows != nil {
			t.Fatal(got, err)
		}
	}
}

type contextReader func(context.Context, session.Statement) (session.Result, error)

func (f contextReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func TestContextReadBudgetsReferenceAndCancellation(t *testing.T) {
	ref, err := reference.NewReference("unit", "context", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := parameters.HiddenContextRequest{OracleMajor: 11, Reference: ref}
	r := contextResult(false)
	sql, args, _ := parameters.HiddenContextStatement(11, request.Filter)
	calls := 0
	reader := contextReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Text != sql || s.Reference != ref || !reflect.DeepEqual(s.Args, args) {
			t.Fatal(s)
		}
		return r, nil
	})
	got, err := parameters.ReadHiddenContext(t.Context(), reader, request)
	if err != nil || len(got.Rows) != 3 || calls != 1 {
		t.Fatal(got, err, calls)
	}
	cause := errors.New("private failed cursor")
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return r, cause }
	got, err = parameters.ReadHiddenContext(t.Context(), reader, request)
	if !errors.Is(err, cause) || got.Rows != nil || calls != 2 || strings.Contains(err.Error(), cause.Error()) {
		t.Fatal(got, err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return r, nil }
	got, err = parameters.ReadHiddenContext(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	reader = func(context.Context, session.Statement) (session.Result, error) { return r, nil }
	for _, limits := range []rowset.Limits{{MaxRows: 2}, {MaxBytes: 1}} {
		request.Limits = limits
		got, err = parameters.ReadHiddenContext(t.Context(), reader, request)
		if !errors.Is(err, rowset.ErrCapacity) || got.Rows != nil {
			t.Fatal(got, err)
		}
	}
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return r, nil }
	request.Limits = rowset.Limits{MaxRows: -1}
	if _, err = parameters.ReadHiddenContext(t.Context(), reader, request); !errors.Is(err, rowset.ErrInvalidRequest) || calls != 0 {
		t.Fatal(err, calls)
	}
}
func TestContextNativeRowsAndJSONKeepAdmissionAtomic(t *testing.T) {
	step := contextStep(true)
	db, state := testsql.New(t, step)
	rows, err := parameters.HiddenContextRows(t.Context(), db, 19, parameters.HiddenContextFilter{})
	if err != nil || rows == nil || state.Closed() != 0 || !rows.Next() {
		t.Fatal(rows, err)
	}
	if err = rows.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	terminal := errors.New("terminal")
	closeErr := errors.New("close")
	for _, tc := range []struct {
		name   string
		change func(*testsql.Step)
		limits rowset.Limits
		want   error
	}{
		{"terminal", func(s *testsql.Step) { s.NextErr = terminal }, rowset.Limits{}, terminal}, {"close", func(s *testsql.Step) { s.CloseErr = closeErr }, rowset.Limits{}, closeErr}, {"rows", func(*testsql.Step) {}, rowset.Limits{MaxRows: 2}, rowset.ErrCapacity}, {"bytes", func(*testsql.Step) {}, rowset.Limits{MaxBytes: 1}, rowset.ErrCapacity}, {"late-scope", func(s *testsql.Step) { s.Values[2][5] = "OTHER" }, rowset.Limits{}, parameters.ErrInvalidEvidence}, {"late-number", func(s *testsql.Step) { s.Values[2][4] = "0.5" }, rowset.Limits{}, rowset.ErrInvalidEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := contextStep(true)
			tc.change(&s)
			db, _ := testsql.New(t, s)
			filter := parameters.HiddenContextFilter{}
			if tc.name == "late-scope" {
				filter.Names = []string{"compatible", "_example"}
			}
			data, err := parameters.HiddenContextJSON(t.Context(), db, 19, filter, tc.limits)
			if !errors.Is(err, tc.want) || data != nil {
				t.Fatal(string(data), err)
			}
		})
	}
	for _, bad := range []driver.Value{nil, "TRUE"} {
		s := contextStep(true)
		s.Values = s.Values[:1]
		s.Values[0][7] = bad
		db, _ := testsql.New(t, s)
		data, err := parameters.HiddenContextJSON(t.Context(), db, 19, parameters.HiddenContextFilter{NonDefaultOnly: true}, rowset.Limits{})
		if !errors.Is(err, parameters.ErrInvalidEvidence) || data != nil {
			t.Fatal(string(data), err)
		}
	}
	db, _ = testsql.New(t, step)
	data, err := parameters.HiddenContextJSON(t.Context(), db, 19, parameters.HiddenContextFilter{}, rowset.Limits{})
	var envelope struct {
		Provenance parameters.HiddenContextProvenance
		Data       struct{ Rows [][]any }
	}
	if err != nil || json.Unmarshal(data, &envelope) != nil || len(envelope.Data.Rows) != 3 || envelope.Data.Rows[1][6] != nil || envelope.Data.Rows[0][5] != "compatible" {
		t.Fatal(string(data), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	s := contextStep(true)
	s.OnNext = func(int) { cancel() }
	db, state = testsql.New(t, s)
	data, err = parameters.HiddenContextJSON(ctx, db, 19, parameters.HiddenContextFilter{}, rowset.Limits{})
	if !errors.Is(err, context.Canceled) || data != nil || state.Closed() != 1 {
		t.Fatal(string(data), err, state.Closed())
	}
}
