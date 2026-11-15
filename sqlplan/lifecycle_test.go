package sqlplan

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

type fixtureReader func(context.Context, session.Statement) (session.Result, error)

func (f fixtureReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func fixtureResult(t *testing.T) (Options, selection, session.Result) {
	t.Helper()
	options := Options{OracleMajor: 19}
	plan, err := profilePlan(options)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Result{Columns: make([]string, len(plan.schema)), Rows: [][]session.Value{make([]session.Value, len(plan.schema))}}
	for i, c := range plan.schema {
		result.Columns[i] = c.Name
		result.Rows[0][i] = session.Value{Text: "raw \ntext"}
		for _, name := range plan.integers {
			if name == c.Name {
				result.Rows[0][i] = session.Value{Text: "1234567890123456789012345678"}
			}
		}
	}
	return options, plan, result
}
func TestReadRejectsFailureCancellationAndFiniteBudget(t *testing.T) {
	options, plan, result := fixtureResult(t)
	ref, err := reference.NewReference("unit", "facts", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Options: options, Reference: ref}
	calls := 0
	reader := fixtureReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref || s.Text != plan.sql {
			t.Fatal("statement/reference changed")
		}
		return result, nil
	})
	got, err := ReadProfile(t.Context(), reader, request)
	if err != nil || len(got.Records) != 1 || calls != 1 {
		t.Fatal(got, err, calls)
	}
	cause := errors.New("private connection path and binding")
	reader = func(context.Context, session.Statement) (session.Result, error) { return result, cause }
	got, err = ReadProfile(t.Context(), reader, request)
	if !errors.Is(err, cause) || got.Records != nil || strings.Contains(err.Error(), cause.Error()) {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return result, nil }
	got, err = ReadProfile(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Records != nil {
		t.Fatal(got, err)
	}
	reader = func(context.Context, session.Statement) (session.Result, error) { return result, nil }
	request.Limits = rowset.Limits{MaxBytes: 1, MaxRows: 1}
	got, err = ReadProfile(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || got.Records != nil {
		t.Fatal(got, err)
	}
	request.Limits = rowset.Limits{MaxBytes: 65536, MaxRows: 1}
	result.Rows = append(result.Rows, result.Rows[0])
	got, err = ReadProfile(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || got.Records != nil {
		t.Fatal(got, err)
	}
	var nilReader *nilFactReader
	request.Reference = ref
	_, err = ReadProfile(t.Context(), nilReader, request)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}

type nilFactReader struct{}

func (*nilFactReader) Query(context.Context, session.Statement) (session.Result, error) {
	panic("typed nil called")
}
func TestDecodeNeverPublishesMalformedLatePrefix(t *testing.T) {
	for _, kind := range []string{"missing-column", "duplicate-column", "late-width", "late-integer", "late-invalid-utf8", "null-with-text"} {
		t.Run(kind, func(t *testing.T) {
			options, _, r := fixtureResult(t)
			r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...))
			switch kind {
			case "missing-column":
				r.Columns = r.Columns[1:]
			case "duplicate-column":
				r.Columns[0] = r.Columns[1]
			case "late-width":
				r.Rows[1] = r.Rows[1][:1]
			case "late-integer":
				changed := false
				for j, name := range r.Columns {
					if name == "USER_ID" || name == "CACHE_SIZE" || name == "LOWER_PORT" || name == "SIGNATURE" {
						r.Rows[1][j] = session.Value{Text: "1.5"}
						changed = true
						break
					}
				}
				if !changed {
					r.Rows[1] = r.Rows[1][:1]
				}
			case "late-invalid-utf8":
				r.Rows[1][0] = session.Value{Text: string([]byte{255})}
			case "null-with-text":
				r.Rows[1][0] = session.Value{Text: "x", Null: true}
			}
			got, err := DecodeProfile(r, options)
			if !errors.Is(err, ErrInvalidEvidence) || got.Records != nil {
				t.Fatal(got, err)
			}
		})
	}
	options, _, r := fixtureResult(t)
	r.Rows = nil
	got, err := DecodeProfile(r, options)
	if err != nil || got.Records == nil || len(got.Records) != 0 || got.Provenance.Source == "" {
		t.Fatal(got, err)
	}
}
func TestNativeCursorAndJSONFailureOwnership(t *testing.T) {
	options, plan, r := fixtureResult(t)
	values := make([]driver.Value, len(r.Columns))
	for i, v := range r.Rows[0] {
		values[i] = v.Text
	}
	// A native dataset is the actual database/sql cursor: no preconsumption/close.
	db, state := testsql.New(t, testsql.Step{SQL: plan.sql, Columns: r.Columns, Values: [][]driver.Value{values}})
	cursor, err := ProfileRows(t.Context(), db, options)
	if err != nil || cursor == nil || state.Closed() != 0 {
		t.Fatal(cursor, err, state.Closed())
	}
	if !cursor.Next() {
		t.Fatal(cursor.Err())
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	terminal := errors.New("terminal")
	closeErr := errors.New("close")
	for _, tc := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{
		{"terminal", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, NextErr: terminal}, rowset.Limits{}, terminal},
		{"close", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, CloseErr: closeErr}, rowset.Limits{}, closeErr},
		{"late-type", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values, append([]driver.Value{true}, values[1:]...)}}, rowset.Limits{}, rowset.ErrInvalidEvidence},
		{"row-budget", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity},
		{"byte-budget", testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 1}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			data, err := ProfileJSON(t.Context(), db, options, tc.limits)
			if !errors.Is(err, tc.want) || data != nil {
				t.Fatal(string(data), err)
			}
			if tc.name != "byte-budget" && state.Closed() != 1 {
				t.Fatal("cursor not finalized", state.Closed())
			}
		})
	}
	db, _ = testsql.New(t, testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}})
	data, err := ProfileJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || !json.Valid(data) || !strings.Contains(string(data), "provenance") {
		t.Fatal(string(data), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	db, state = testsql.New(t, testsql.Step{Columns: r.Columns, Values: [][]driver.Value{values}, OnNext: func(int) { cancel() }})
	data, err = ProfileJSON(ctx, db, options, rowset.Limits{})
	if !errors.Is(err, context.Canceled) || data != nil || state.Closed() != 1 {
		t.Fatal(data, err, state.Closed())
	}
}
