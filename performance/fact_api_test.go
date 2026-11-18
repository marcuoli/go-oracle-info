package performance

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

type apiReader func(context.Context, session.Statement) (session.Result, error)

func (f apiReader) Query(c context.Context, s session.Statement) (session.Result, error) {
	return f(c, s)
}

type apiSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *apiSpy) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	r, e := s.db.QueryContext(c, q, a...)
	s.last = r
	return r, e
}

func TestBlockersCompleteAPI(t *testing.T) {
	options := BlockersOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "performance", "Blockers", 1)
	cols := []string{"SID", "SERIAL_NUMBER", "BLOCKING_INSTANCE", "BLOCKING_SESSION", "EVENT"}
	values := []driver.Value{"7", "9007199254740993", "2", "5", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "7"}, {Text: "9007199254740993"}, {Text: "2"}, {Text: "5"}, {Null: true}}, {{Text: "7"}, {Text: "9007199254740993"}, {Text: "2"}, {Text: "5"}, {Null: true}}}}
	decoded, e := DecodeBlockers(input, options)
	if e != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Blocker{SID: session.Value{Text: "7"}, SerialNumber: session.Value{Text: "9007199254740993"}, BlockingInstance: session.Value{Text: "2"}, BlockingSession: session.Value{Text: "5"}, Event: session.Value{Null: true}}) {
		t.Fatal(decoded, e)
	}
	calls := 0
	reader := apiReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, e := ReadBlockers(t.Context(), reader, BlockersRequest{Options: options, Reference: ref})
	if e != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, e, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, e := DecodeBlockers(bad, options)
	if !errors.Is(e, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, e = ReadBlockers(ctx, reader, BlockersRequest{Options: options, Reference: ref})
	if !errors.Is(e, context.Canceled) || got.Rows != nil {
		t.Fatal(got, e)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, e = ReadBlockers(t.Context(), reader, BlockersRequest{Options: options, Reference: ref})
	if !errors.Is(e, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(e, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, e := BlockersRows(t.Context(), spy, options)
	if e != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, e)
	}
	if e = cursor.Close(); e != nil || state.Closed() != 1 {
		t.Fatal(e, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, e := BlockersJSON(t.Context(), db, options, rowset.Limits{})
	if e != nil || state.Closed() != 1 {
		t.Fatal(string(payload), e)
	}
	var document struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Columns []rowset.Column `json:"columns"`
			Rows    [][]any         `json:"rows"`
		} `json:"data"`
	}
	if e = json.Unmarshal(payload, &document); e != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"7", "9007199254740993", "2", "5", nil}, {"7", "9007199254740993", "2", "5", nil}}) {
		t.Fatal(string(payload), document, e)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, e := BlockersJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(e, failure.want) || b != nil {
				t.Fatal(string(b), e)
			}
		})
	}
}

func TestResourceLimitsCompleteAPI(t *testing.T) {
	options := ResourceLimitsOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "performance", "ResourceLimits", 1)
	cols := []string{"RESOURCE_NAME", "CURRENT_UTILIZATION", "MAX_UTILIZATION", "LIMIT_VALUE"}
	values := []driver.Value{"processes", "9007199254740993", "9223372036854775808", "UNLIMITED"}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "processes"}, {Text: "9007199254740993"}, {Text: "9223372036854775808"}, {Text: "UNLIMITED"}}, {{Text: "processes"}, {Text: "9007199254740993"}, {Text: "9223372036854775808"}, {Text: "UNLIMITED"}}}}
	decoded, e := DecodeResourceLimits(input, options)
	if e != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ResourceLimit{Name: session.Value{Text: "processes"}, CurrentUtilization: session.Value{Text: "9007199254740993"}, MaxUtilization: session.Value{Text: "9223372036854775808"}, Limit: session.Value{Text: "UNLIMITED"}}) {
		t.Fatal(decoded, e)
	}
	calls := 0
	reader := apiReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, e := ReadResourceLimits(t.Context(), reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if e != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, e, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, e := DecodeResourceLimits(bad, options)
	if !errors.Is(e, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, e = ReadResourceLimits(ctx, reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if !errors.Is(e, context.Canceled) || got.Rows != nil {
		t.Fatal(got, e)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, e = ReadResourceLimits(t.Context(), reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if !errors.Is(e, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(e, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, e := ResourceLimitsRows(t.Context(), spy, options)
	if e != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, e)
	}
	if e = cursor.Close(); e != nil || state.Closed() != 1 {
		t.Fatal(e, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, e := ResourceLimitsJSON(t.Context(), db, options, rowset.Limits{})
	if e != nil || state.Closed() != 1 {
		t.Fatal(string(payload), e)
	}
	var document struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Columns []rowset.Column `json:"columns"`
			Rows    [][]any         `json:"rows"`
		} `json:"data"`
	}
	if e = json.Unmarshal(payload, &document); e != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"processes", "9007199254740993", "9223372036854775808", "UNLIMITED"}, {"processes", "9007199254740993", "9223372036854775808", "UNLIMITED"}}) {
		t.Fatal(string(payload), document, e)
	}
	for _, failure := range []struct {
		name   string
		step   testsql.Step
		limits rowset.Limits
		want   error
	}{{"terminal", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, NextErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"close", testsql.Step{Columns: cols, Values: [][]driver.Value{values}, CloseErr: context.DeadlineExceeded}, rowset.Limits{}, context.DeadlineExceeded}, {"rows", testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}}, rowset.Limits{MaxRows: 1}, rowset.ErrCapacity}, {"bytes", testsql.Step{Columns: cols, Values: [][]driver.Value{values}}, rowset.Limits{MaxBytes: 8}, rowset.ErrCapacity}} {
		t.Run(failure.name, func(t *testing.T) {
			db, _ := testsql.New(t, failure.step)
			b, e := ResourceLimitsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(e, failure.want) || b != nil {
				t.Fatal(string(b), e)
			}
		})
	}
}
