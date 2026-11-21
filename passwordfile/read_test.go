package passwordfile_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/passwordfile"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

var baseOptions = passwordfile.Options{OracleVersion: "19.32.0.0.0"}

func metadataResult(extra bool) session.Result {
	result := session.Result{Columns: []string{"FILE_NAME", "FORMAT", "IS_ASM"}, Rows: [][]session.Value{{{Text: " /dbs/orapwDEMO "}, {Text: "future"}, {Text: "FALSE"}}}}
	if extra {
		result.Columns = append(result.Columns, "IS_EXC")
		result.Rows[0] = append(result.Rows[0], session.Value{Null: true})
	}
	result.Columns = append(result.Columns, "CON_ID")
	result.Rows[0] = append(result.Rows[0], session.Value{Text: "0"})
	return result
}
func metadataReference(t *testing.T) reference.Reference {
	t.Helper()
	r, e := reference.NewReference("unit", "metadata", "passwordfile", 1)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

type metadataReader func(context.Context, session.Statement) (session.Result, error)

func (f metadataReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func TestMetadataObservationPreservesOptionalNullAndSource(t *testing.T) {
	t.Parallel()
	for _, extra := range []bool{false, true} {
		t.Run(map[bool]string{false: "unselected", true: "selected-null"}[extra], func(t *testing.T) {
			options := baseOptions
			options.IncludeExascale = extra
			result := metadataResult(extra)
			got, err := passwordfile.Decode(result, options)
			if err != nil || got.State != passwordfile.Available || got.Value == nil || got.Value.FileName.Text != " /dbs/orapwDEMO " || got.Value.Format.Text != "future" || got.Value.IsASM.Text != "FALSE" || got.Value.ContainerID.Text != "0" || got.Provenance.Source != "V$PASSWORDFILE_INFO" || got.Provenance.ExascaleColumn != extra {
				t.Fatal(got, err)
			}
			if extra {
				if got.Value.IsExascale == nil || !got.Value.IsExascale.Null {
					t.Fatal("selected NULL lost")
				}
			} else if got.Value.IsExascale != nil {
				t.Fatal("unselected flag invented")
			}
			for i := range result.Rows[0] {
				result.Rows[0][i] = session.Value{Null: true}
			}
			got, err = passwordfile.Decode(result, options)
			if err != nil || got.State != passwordfile.Available || !got.Value.FileName.Null || !got.Value.Format.Null {
				t.Fatal("NULL row inferred absent", got, err)
			}
			result.Rows = nil
			got, err = passwordfile.Decode(result, options)
			if err != nil || got.State != passwordfile.NoRows || got.Value != nil {
				t.Fatal("successful empty conflated", got, err)
			}
		})
	}
	got, err := passwordfile.Decode(metadataResult(false), passwordfile.Options{OracleVersion: "12.1.0.2.0"})
	if !errors.Is(err, passwordfile.ErrUnsupportedVersion) || got.State != passwordfile.Unsupported || got.Value != nil {
		t.Fatal(got, err)
	}
}
func TestMetadataRejectsMalformedWholeObservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"second-row", func(r *session.Result) { r.Rows = append(r.Rows, append([]session.Value(nil), r.Rows[0]...)) }},
		{"bad-width", func(r *session.Result) { r.Rows[0] = r.Rows[0][:1] }},
		{"duplicate-column", func(r *session.Result) { r.Columns[0] = "FORMAT" }},
		{"wrong-container", func(r *session.Result) { r.Rows[0][3] = session.Value{Text: "3"} }},
		{"noninteger-container", func(r *session.Result) { r.Rows[0][3] = session.Value{Text: "0.5"} }},
		{"invalid-utf8", func(r *session.Result) { r.Rows[0][0] = session.Value{Text: string([]byte{255})} }},
		{"null-with-text", func(r *session.Result) { r.Rows[0][0] = session.Value{Text: "private", Null: true} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := metadataResult(false)
			tc.change(&r)
			got, err := passwordfile.Decode(r, baseOptions)
			if !errors.Is(err, passwordfile.ErrInvalidEvidence) || got.State != passwordfile.Unavailable || got.Value != nil {
				t.Fatal(got, err)
			}
		})
	}
}
func TestReadMetadataReferenceFailureAndCancellation(t *testing.T) {
	t.Parallel()
	ref := metadataReference(t)
	request := passwordfile.Request{Options: baseOptions, Reference: ref}
	sql, err := passwordfile.Statement(baseOptions)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	reader := metadataReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref || s.Text != sql || len(s.Args) != 0 || ctx != t.Context() {
			t.Fatal(s)
		}
		return metadataResult(false), nil
	})
	got, err := passwordfile.Read(t.Context(), reader, request)
	if err != nil || got.Value == nil || calls != 1 {
		t.Fatal(got, err, calls)
	}
	failure := errors.New("private file/credential error details")
	reader = func(context.Context, session.Statement) (session.Result, error) {
		return metadataResult(false), failure
	}
	got, err = passwordfile.Read(t.Context(), reader, request)
	if !errors.Is(err, failure) || got.State != passwordfile.Unavailable || got.Value != nil || strings.Contains(err.Error(), failure.Error()) {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return metadataResult(false), nil
	}
	got, err = passwordfile.Read(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || got.Value != nil {
		t.Fatal(got, err)
	}
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) {
		calls++
		return session.Result{}, nil
	}
	if _, err = passwordfile.Read(t.Context(), reader, passwordfile.Request{Options: baseOptions}); !errors.Is(err, passwordfile.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if _, err = passwordfile.Read(ctx, reader, request); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = passwordfile.Read(t.Context(), reader, passwordfile.Request{Options: passwordfile.Options{OracleVersion: "11.2.0.2.0"}, Reference: ref}); !errors.Is(err, passwordfile.ErrUnsupportedVersion) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("invalid/unsupported/cancelled read queried")
	}
}
func metadataStep(extra bool) testsql.Step {
	result := metadataResult(extra)
	values := make([]driver.Value, len(result.Rows[0]))
	for i, v := range result.Rows[0] {
		if !v.Null {
			values[i] = v.Text
		}
	}
	values[len(values)-1] = int64(0)
	return testsql.Step{Columns: result.Columns, Values: [][]driver.Value{values}}
}

type metadataSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *metadataSpy) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	s.last = rows
	return rows, err
}
func TestNativeRowsAreBorrowedAndJSONIsIndependent(t *testing.T) {
	t.Parallel()
	options := baseOptions
	options.IncludeExascale = true
	step := metadataStep(true)
	db, state := testsql.New(t, step, step)
	spy := &metadataSpy{db: db}
	rows, err := passwordfile.Rows(t.Context(), spy, options)
	if err != nil || rows != spy.last || !rows.Next() {
		t.Fatal("native pointer/lifetime changed", err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := passwordfile.JSON(t.Context(), spy, options, rowset.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Provenance passwordfile.Provenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err = dec.Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(state.Calls()) != 2 || state.Calls()[0].SQL != state.Calls()[1].SQL || state.Closed() != 2 || payload.Provenance.ExascaleColumn != true || len(payload.Data.Rows) != 1 || payload.Data.Rows[0][0] != " /dbs/orapwDEMO " || payload.Data.Rows[0][3] != nil || payload.Data.Rows[0][4] != json.Number("0") {
		t.Fatal("native/JSON source or values changed", payload)
	}
}
func TestMetadataJSONIsAtomicAndByteBounded(t *testing.T) {
	t.Parallel()
	step := metadataStep(false)
	db, _ := testsql.New(t, step, step, step)
	data, err := passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	exact, err := passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{MaxBytes: len(data)})
	if err != nil || string(exact) != string(data) {
		t.Fatal("exact byte budget refused", err)
	}
	data, err = passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{MaxBytes: len(exact) - 1})
	if !errors.Is(err, rowset.ErrCapacity) || data != nil {
		t.Fatal("overbudget success/prefix", string(data), err)
	}
	failure := errors.New("private terminal failure")
	for _, tc := range []struct {
		name string
		step testsql.Step
	}{{"terminal", func() testsql.Step { s := step; s.NextErr = failure; return s }()}, {"close", func() testsql.Step { s := step; s.CloseErr = failure; return s }()}, {"two-rows", func() testsql.Step { s := step; s.Values = append(s.Values, s.Values[0]); return s }()}} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, tc.step)
			got, err := passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{})
			if err == nil || got != nil || strings.Contains(err.Error(), failure.Error()) {
				t.Fatal("failed metadata escaped", string(got), err)
			}
			if tc.name == "two-rows" {
				var domain *passwordfile.Error
				if !errors.As(err, &domain) || domain.Operation != "consume" ||
					!errors.Is(err, rowset.ErrCapacity) || state.Closed() != 1 {
					t.Fatalf("metadata row cap = %v, closed = %d", err, state.Closed())
				}
			}
		})
	}
	db, state := testsql.New(t)
	if _, err = passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{MaxBytes: -1}); err == nil || len(state.Calls()) != 0 {
		t.Fatal("invalid budget queried", err)
	}
}
