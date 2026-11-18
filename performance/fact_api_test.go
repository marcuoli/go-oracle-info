package performance

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
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
	r, err := s.db.QueryContext(c, q, a...)
	s.last = r
	return r, err
}

func TestBlockersCompleteAPI(t *testing.T) {
	options := BlockersOptions{OracleMajor: 19}
	ref, _ := reference.NewReference("unit", "performance", "Blockers", 1)
	cols := []string{"SID", "SERIAL_NUMBER", "BLOCKING_INSTANCE", "BLOCKING_SESSION", "EVENT"}
	values := []driver.Value{"7", "9007199254740993", "2", "5", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "7"}, {Text: "9007199254740993"}, {Text: "2"}, {Text: "5"}, {Null: true}}, {{Text: "7"}, {Text: "9007199254740993"}, {Text: "2"}, {Text: "5"}, {Null: true}}}}
	decoded, err := DecodeBlockers(input, options)
	if err != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Blocker{SID: session.Value{Text: "7"}, SerialNumber: session.Value{Text: "9007199254740993"}, BlockingInstance: session.Value{Text: "2"}, BlockingSession: session.Value{Text: "5"}, Event: session.Value{Null: true}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := apiReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadBlockers(t.Context(), reader, BlockersRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, err := DecodeBlockers(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadBlockers(ctx, reader, BlockersRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadBlockers(t.Context(), reader, BlockersRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, err := BlockersRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, err := BlockersJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Columns []rowset.Column `json:"columns"`
			Rows    [][]any         `json:"rows"`
		} `json:"data"`
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"7", "9007199254740993", "2", "5", nil}, {"7", "9007199254740993", "2", "5", nil}}) {
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
			b, err := BlockersJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
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
	decoded, err := DecodeResourceLimits(input, options)
	if err != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], ResourceLimit{Name: session.Value{Text: "processes"}, CurrentUtilization: session.Value{Text: "9007199254740993"}, MaxUtilization: session.Value{Text: "9223372036854775808"}, Limit: session.Value{Text: "UNLIMITED"}}) {
		t.Fatal(decoded, err)
	}
	calls := 0
	reader := apiReader(func(c context.Context, s session.Statement) (session.Result, error) {
		calls++
		if s.Reference != ref {
			t.Fatal(s.Reference)
		}
		return input, nil
	})
	facts, err := ReadResourceLimits(t.Context(), reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, err := DecodeResourceLimits(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadResourceLimits(ctx, reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadResourceLimits(t.Context(), reader, ResourceLimitsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, err := ResourceLimitsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, err := ResourceLimitsJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || state.Closed() != 1 {
		t.Fatal(string(payload), err)
	}
	var document struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Columns []rowset.Column `json:"columns"`
			Rows    [][]any         `json:"rows"`
		} `json:"data"`
	}
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"processes", "9007199254740993", "9223372036854775808", "UNLIMITED"}, {"processes", "9007199254740993", "9223372036854775808", "UNLIMITED"}}) {
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
			b, err := ResourceLimitsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
