package dataguard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"reflect"
	"testing"
)

type datasetTestReader func(context.Context, session.Statement) (session.Result, error)

func (f datasetTestReader) Query(c context.Context, s session.Statement) (session.Result, error) {
	return f(c, s)
}

type datasetSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *datasetSpy) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(c, q, a...)
	s.last = r
	return r, e
}

func TestDatasetStatusCompleteAPI(t *testing.T) {
	options := StatusOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "dataguard", "Status", 1)
	cols := []string{"DATABASE_ROLE", "PROTECTION_MODE", "PROTECTION_LEVEL", "SWITCHOVER_STATUS"}
	values := []driver.Value{"PHYSICAL STANDBY", "MAXIMUM PERFORMANCE", "RESYNCHRONIZATION", "NOT ALLOWED"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "PHYSICAL STANDBY"}, {Text: "MAXIMUM PERFORMANCE"}, {Text: "RESYNCHRONIZATION"}, {Text: "NOT ALLOWED"}}}}
	decoded, err := DecodeStatus(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], StatusRow{Role: session.Value{Text: "PHYSICAL STANDBY"}, ProtectionMode: session.Value{Text: "MAXIMUM PERFORMANCE"}, ProtectionLevel: session.Value{Text: "RESYNCHRONIZATION"}, SwitchoverStatus: session.Value{Text: "NOT ALLOWED"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadStatus(t.Context(), reader, StatusRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeStatus(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadStatus(ctx, reader, StatusRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadStatus(t.Context(), reader, StatusRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadStatus(t.Context(), reader, StatusRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := StatusRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := StatusJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"PHYSICAL STANDBY", "MAXIMUM PERFORMANCE", "RESYNCHRONIZATION", "NOT ALLOWED"}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := StatusJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestDatasetLagCompleteAPI(t *testing.T) {
	options := LagOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "dataguard", "Lag", 1)
	cols := []string{"NAME", "VALUE", "UNIT", "TIME_COMPUTED", "DATUM_TIME"}
	values := []driver.Value{"apply lag", nil, "day(2) to second(0) interval", "11/18/2026 12:34:56", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "apply lag"}, {Null: true}, {Text: "day(2) to second(0) interval"}, {Text: "11/18/2026 12:34:56"}, {Null: true}}}}
	decoded, err := DecodeLag(input, options)
	if err != nil || len(decoded.Rows) != 1 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], LagRow{Name: session.Value{Text: "apply lag"}, Value: session.Value{Null: true}, Unit: session.Value{Text: "day(2) to second(0) interval"}, TimeComputed: session.Value{Text: "11/18/2026 12:34:56"}, DatumTime: session.Value{Null: true}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := datasetTestReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadLag(t.Context(), reader, LagRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "late wrong width"}, {Text: "surplus"}, {Text: "surplus"}}}
	bad.Columns = append([]string(nil), cols...)
	bad.Columns[len(cols)-1] = "UNEXPECTED"
	got, err := DecodeLag(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadLag(ctx, reader, LagRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("private driver detail")
	reader = func(context.Context, session.Statement) (session.Result, error) { return input, cause }
	got, err = ReadLag(t.Context(), reader, LagRequest{Options: options, Reference: ref})
	if !errors.Is(err, cause) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadLag(t.Context(), reader, LagRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	spy := &datasetSpy{db: db}
	cursor, err := LagRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values}})
	payload, err := LagJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance DatasetProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"apply lag", nil, "day(2) to second(0) interval", "11/18/2026 12:34:56", nil}}) {
		t.Fatal(string(payload), document, err)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, err := LagJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
