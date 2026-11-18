package scheduler

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

func TestJobsCompleteAPI(t *testing.T) {
	options := JobsOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "scheduler", "Jobs", 1)
	cols := []string{"OWNER", "JOB_NAME", "ENABLED", "STATE", "FAILURE_COUNT", "NEXT_RUN_DATE"}
	values := []driver.Value{"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "CaseOwner"}, {Text: "Job雪"}, {Text: "FALSE"}, {Text: "DISABLED"}, {Text: "9007199254740993"}, {Null: true}}, {{Text: "CaseOwner"}, {Text: "Job雪"}, {Text: "FALSE"}, {Text: "DISABLED"}, {Text: "9007199254740993"}, {Null: true}}}}
	decoded, e := DecodeJobs(input, options)
	if e != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Job{Owner: session.Value{Text: "CaseOwner"}, Name: session.Value{Text: "Job雪"}, Enabled: session.Value{Text: "FALSE"}, State: session.Value{Text: "DISABLED"}, FailureCount: session.Value{Text: "9007199254740993"}, NextRunDate: session.Value{Null: true}}) {
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
	facts, e := ReadJobs(t.Context(), reader, JobsRequest{Options: options, Reference: ref})
	if e != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, e, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, e := DecodeJobs(bad, options)
	if !errors.Is(e, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, e = ReadJobs(ctx, reader, JobsRequest{Options: options, Reference: ref})
	if !errors.Is(e, context.Canceled) || got.Rows != nil {
		t.Fatal(got, e)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, e = ReadJobs(t.Context(), reader, JobsRequest{Options: options, Reference: ref})
	if !errors.Is(e, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(e, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, e := JobsRows(t.Context(), spy, options)
	if e != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, e)
	}
	if e = cursor.Close(); e != nil || state.Closed() != 1 {
		t.Fatal(e, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, e := JobsJSON(t.Context(), db, options, rowset.Limits{})
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
	if e = json.Unmarshal(payload, &document); e != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}, {"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}}) {
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
			b, e := JobsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(e, failure.want) || b != nil {
				t.Fatal(string(b), e)
			}
		})
	}
}
