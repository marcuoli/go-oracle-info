package scheduler

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

func TestJobsCompleteAPI(t *testing.T) {
	options := JobsOptions{OracleMajor: 19, Owner: "CaseOwner"}
	ref, _ := reference.NewReference("unit", "scheduler", "Jobs", 1)
	cols := []string{"OWNER", "JOB_NAME", "ENABLED", "STATE", "FAILURE_COUNT", "NEXT_RUN_DATE"}
	values := []driver.Value{"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}
	input := session.Result{Columns: cols, Rows: [][]session.Value{{{Text: "CaseOwner"}, {Text: "Job雪"}, {Text: "FALSE"}, {Text: "DISABLED"}, {Text: "9007199254740993"}, {Null: true}}, {{Text: "CaseOwner"}, {Text: "Job雪"}, {Text: "FALSE"}, {Text: "DISABLED"}, {Text: "9007199254740993"}, {Null: true}}}}
	decoded, err := DecodeJobs(input, options)
	if err != nil || len(decoded.Rows) != 2 || decoded.Provenance.Complete || !reflect.DeepEqual(decoded.Rows[0], Job{Owner: session.Value{Text: "CaseOwner"}, Name: session.Value{Text: "Job雪"}, Enabled: session.Value{Text: "FALSE"}, State: session.Value{Text: "DISABLED"}, FailureCount: session.Value{Text: "9007199254740993"}, NextRunDate: session.Value{Null: true}}) {
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
	facts, err := ReadJobs(t.Context(), reader, JobsRequest{Options: options, Reference: ref})
	if err != nil || !facts.Provenance.Complete || calls != 1 || !reflect.DeepEqual(facts.Rows, decoded.Rows) {
		t.Fatal(facts, err, calls)
	}
	bad := input
	bad.Rows = [][]session.Value{input.Rows[0], {{Text: "broken width"}}}
	got, err := DecodeJobs(bad, options)
	if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	reader = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	got, err = ReadJobs(ctx, reader, JobsRequest{Options: options, Reference: ref})
	if !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatal(got, err)
	}
	options.OracleMajor = 10
	calls = 0
	reader = func(context.Context, session.Statement) (session.Result, error) { calls++; return input, nil }
	_, err = ReadJobs(t.Context(), reader, JobsRequest{Options: options, Reference: ref})
	if !errors.Is(err, ErrUnsupportedVersion) || calls != 0 {
		t.Fatal(err, calls)
	}
	options.OracleMajor = 19
	db, state := testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	spy := &apiSpy{db: db}
	cursor, err := JobsRows(t.Context(), spy, options)
	if err != nil || cursor != spy.last || state.Closed() != 0 || !cursor.Next() {
		t.Fatal(cursor, err)
	}
	if err = cursor.Close(); err != nil || state.Closed() != 1 {
		t.Fatal(err, state.Closed())
	}
	db, state = testsql.New(t, testsql.Step{Columns: cols, Values: [][]driver.Value{values, values}})
	payload, err := JobsJSON(t.Context(), db, options, rowset.Limits{})
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
	if err = json.Unmarshal(payload, &document); err != nil || !document.Provenance.Complete || !reflect.DeepEqual(document.Data.Rows, [][]any{{"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}, {"CaseOwner", "Job雪", "FALSE", "DISABLED", "9007199254740993", nil}}) {
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
			b, err := JobsJSON(t.Context(), db, options, failure.limits)
			if !errors.Is(err, failure.want) || b != nil {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestJobsJSONRejectsLateOwnerMismatch(t *testing.T) {
	db, state := testsql.New(t, testsql.Step{
		Columns: []string{"OWNER", "JOB_NAME", "ENABLED", "STATE", "FAILURE_COUNT", "NEXT_RUN_DATE"},
		Values: [][]driver.Value{
			{"CaseOwner", "first_job", "TRUE", "SCHEDULED", "0", nil},
			{"CASEOWNER", "other_job", "TRUE", "SCHEDULED", "0", nil},
		},
	})
	payload, err := JobsJSON(t.Context(), db, JobsOptions{OracleMajor: 19, Owner: "CaseOwner"}, rowset.Limits{})
	var failure *Error
	if payload != nil || !errors.Is(err, ErrInvalidEvidence) ||
		!errors.As(err, &failure) || failure.Source != "DBA_SCHEDULER_JOBS" || state.Closed() != 1 {
		t.Fatalf("jobs JSON = %s, err = %v, closed = %d", payload, err, state.Closed())
	}
}
