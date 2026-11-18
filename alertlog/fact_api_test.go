package alertlog

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

func TestEntriesCompleteAPI(t *testing.T) {
	options := EntriesOptions{OracleMajor: 19, Limit: 2, WindowSeconds: 60}
	ref, _ := reference.NewReference("unit", "alertlog", "Entries", 1)
	cols := []string{"ORIGINATING_TIMESTAMP", "MESSAGE_TYPE", "MESSAGE_LEVEL", "MESSAGE_TEXT"}
	values := []driver.Value{"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "2026-11-18T12:34:56.000000000-03:00"}, {Text: "3"}, {Text: "8"}, {Null: true}}, {{Text: "2026-11-18T12:34:56.000000000-03:00"}, {Text: "3"}, {Text: "8"}, {Null: true}}}}
	decoded, e := DecodeEntries(input, options)
	if e != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Entry{Timestamp: session.Value{Text: "2026-11-18T12:34:56.000000000-03:00"}, MessageType: session.Value{Text: "3"}, MessageLevel: session.Value{Text: "8"}, Message: session.Value{Null: true}}) {
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
	facts, e := ReadEntries(t.Context(), reader, EntriesRequest{Options: options, Reference: ref})
	if e != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, e, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, e := DecodeEntries(bad, options)
	if !errors.Is(e, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, e = ReadEntries(ctx, reader, EntriesRequest{Options: options, Reference: ref})
	if !errors.Is(e, context.Canceled) || got.Rows != nil {
		t.Fatal(got, e)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, e = ReadEntries(t.Context(), reader, EntriesRequest{Options: options, Reference: ref})
	if !errors.Is(e, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(e, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, e := EntriesRows(t.Context(), spy, options)
	if e != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, e)
	}
	if e = cursor.Close(); e != nil || state.Closed() != 1 {
		t.Fatal(e, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, e := EntriesJSON(t.Context(), db, options, rowset.Limits{})
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
	if e = json.Unmarshal(payload, &document); e != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}, {"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}}) {
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
			b, e := EntriesJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(e, failure.want) || b != nil {
				t.Fatal(string(b), e)
			}
		})
	}
}

func TestCountsCompleteAPI(t *testing.T) {
	options := CountsOptions{OracleMajor: 19, WindowSeconds: 60}
	ref, _ := reference.NewReference("unit", "alertlog", "Counts", 1)
	cols := []string{"MESSAGE_LEVEL", "MESSAGE_TYPE", "MESSAGE_COUNT", "FIRST_SEEN", "LAST_SEEN"}
	values := []driver.Value{"8", "3", "9007199254740993", "2026-11-18T12:34:56.000000000-03:00", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "8"}, {Text: "3"}, {Text: "9007199254740993"}, {Text: "2026-11-18T12:34:56.000000000-03:00"}, {Null: true}}, {{Text: "8"}, {Text: "3"}, {Text: "9007199254740993"}, {Text: "2026-11-18T12:34:56.000000000-03:00"}, {Null: true}}}}
	decoded, e := DecodeCounts(input, options)
	if e != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Count{MessageLevel: session.Value{Text: "8"}, MessageType: session.Value{Text: "3"}, MessageCount: session.Value{Text: "9007199254740993"}, FirstSeen: session.Value{Text: "2026-11-18T12:34:56.000000000-03:00"}, LastSeen: session.Value{Null: true}}) {
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
	facts, e := ReadCounts(t.Context(), reader, CountsRequest{Options: options, Reference: ref})
	if e != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, e, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, e := DecodeCounts(bad, options)
	if !errors.Is(e, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, e = ReadCounts(ctx, reader, CountsRequest{Options: options, Reference: ref})
	if !errors.Is(e, context.Canceled) || got.Rows != nil {
		t.Fatal(got, e)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, e = ReadCounts(t.Context(), reader, CountsRequest{Options: options, Reference: ref})
	if !errors.Is(e, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(e, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, e := CountsRows(t.Context(), spy, options)
	if e != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, e)
	}
	if e = cursor.Close(); e != nil || state.Closed() != 1 {
		t.Fatal(e, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, e := CountsJSON(t.Context(), db, options, rowset.Limits{})
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
	if e = json.Unmarshal(payload, &document); e != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"8", "3", "9007199254740993", "2026-11-18T12:34:56.000000000-03:00", nil}, {"8", "3", "9007199254740993", "2026-11-18T12:34:56.000000000-03:00", nil}}) {
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
			b, e := CountsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(e, failure.want) || b != nil {
				t.Fatal(string(b), e)
			}
		})
	}
}
