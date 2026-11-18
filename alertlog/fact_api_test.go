package alertlog

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

func TestEntriesCompleteAPI(t *testing.T) {
	options := EntriesOptions{OracleMajor: 19, Limit: 2, WindowSeconds: 60}
	ref, _ := reference.NewReference("unit", "alertlog", "Entries", 1)
	cols := []string{"ORIGINATING_TIMESTAMP", "MESSAGE_TYPE", "MESSAGE_LEVEL", "MESSAGE_TEXT"}
	values := []driver.Value{"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "2026-11-18T12:34:56.000000000-03:00"}, {Text: "3"}, {Text: "8"}, {Null: true}}, {{Text: "2026-11-18T12:34:56.000000000-03:00"}, {Text: "3"}, {Text: "8"}, {Null: true}}}}
	decoded, err := DecodeEntries(input, options)
	if err != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Entry{Timestamp: session.Value{Text: "2026-11-18T12:34:56.000000000-03:00"}, MessageType: session.Value{Text: "3"}, MessageLevel: session.Value{Text: "8"}, Message: session.Value{Null: true}}) {
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
	facts, err := ReadEntries(t.Context(), reader, EntriesRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, err := DecodeEntries(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadEntries(ctx, reader, EntriesRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadEntries(t.Context(), reader, EntriesRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, err := EntriesRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, err := EntriesJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}, {"2026-11-18T12:34:56.000000000-03:00", "3", "8", nil}}) {
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
			b, err := EntriesJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
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
	decoded, err := DecodeCounts(input, options)
	if err != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Count{MessageLevel: session.Value{Text: "8"}, MessageType: session.Value{Text: "3"}, MessageCount: session.Value{Text: "9007199254740993"}, FirstSeen: session.Value{Text: "2026-11-18T12:34:56.000000000-03:00"}, LastSeen: session.Value{Null: true}}) {
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
	facts, err := ReadCounts(t.Context(), reader, CountsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, err := DecodeCounts(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadCounts(ctx, reader, CountsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadCounts(t.Context(), reader, CountsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, err := CountsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, err := CountsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"8", "3", "9007199254740993", "2026-11-18T12:34:56.000000000-03:00", nil}, {"8", "3", "9007199254740993", "2026-11-18T12:34:56.000000000-03:00", nil}}) {
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
			b, err := CountsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}
