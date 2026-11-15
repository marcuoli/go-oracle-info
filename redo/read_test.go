package redo

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
	"strings"
	"testing"
)

type fixtureReader struct {
	result session.Result
	err    error
	after  func()
	calls  []session.Statement
}

func (r *fixtureReader) Query(_ context.Context, s session.Statement) (session.Result, error) {
	r.calls = append(r.calls, s)
	if r.after != nil {
		r.after()
	}
	return r.result, r.err
}
func sourceResult(plan selection) session.Result {
	result := session.Result{Columns: make([]string, len(plan.schema)), Rows: [][]session.Value{make([]session.Value, len(plan.schema))}}
	for i, c := range plan.schema {
		result.Columns[i] = c.Name
		v := "text"
		if c.Kind == rowset.Integer {
			v = "1"
		}
		for _, n := range plan.integerText {
			if n == c.Name {
				v = "18446744073709551615"
			}
		}
		switch c.Name {
		case "DATA_TYPE":
			v = "LONG RAW"
		case "PROPERTY_NAME":
			v = "DEFAULT_TEMP_TABLESPACE"
		case "FIRST_TIME", "NEXT_TIME", "RESETLOGS_TIME", "COMPLETION_TIME":
			v = "2026-11-01T01:02:03"
		case "NAME":
			v = "buffer_cache"
		}
		result.Rows[0][i] = session.Value{Text: v}
	}
	return result
}
func sourceStep(plan selection) testsql.Step {
	r := sourceResult(plan)
	values := make([]driver.Value, len(r.Columns))
	for i, v := range r.Rows[0] {
		if !v.Null {
			values[i] = v.Text
		}
	}
	return testsql.Step{SQL: plan.sql, Columns: r.Columns, Values: [][]driver.Value{values}}
}
func sourceRef(t *testing.T) reference.Reference {
	t.Helper()
	r, err := reference.NewReference("unit", "facts", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestJSONCompleteNullAndAtomicFailures(t *testing.T) {
	plan := testPlan(t)
	step := sourceStep(plan)
	// SQL NULL and escaped text must retain their native JSON meaning.
	textColumn := -1
	for i, c := range plan.schema {
		if c.Kind == rowset.Text {
			for _, n := range plan.integerText {
				if n == c.Name {
					continue
				}
			}
			textColumn = i
			break
		}
	}
	if textColumn < 0 {
		t.Fatal("test needs textual source")
	}
	step.Values[0][textColumn] = nil
	db, state := testsql.New(t, step)
	payload, err := testJSON(context.Background(), db, rowset.Limits{MaxRows: 2, MaxBytes: 16384})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if err = json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.Complete || envelope.Provenance.Source != plan.provenance.Source || len(envelope.Data.Rows) != 1 || envelope.Data.Rows[0][textColumn] != nil || state.Closed() != 1 {
		t.Fatalf("lost JSON source/NULL/completion: %s, closes=%d", payload, state.Closed())
	}
	sentinel := errors.New("private driver text must not escape")
	for _, kind := range []string{"next", "close", "rows", "bytes", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			step := sourceStep(plan)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := rowset.Limits{MaxRows: 1, MaxBytes: 16384}
			switch kind {
			case "next":
				step.NextErr = sentinel
			case "close":
				step.CloseErr = sentinel
			case "rows":
				step.Values = append(step.Values, step.Values[0])
			case "bytes":
				limits.MaxBytes = 32
			case "cancel":
				step.OnNext = func(i int) {
					if i == 1 {
						cancel()
					}
				}
			}
			db, state := testsql.New(t, step)
			payload, err := testJSON(ctx, db, limits)
			if err == nil || payload != nil {
				t.Fatalf("published failed prefix: %s, %v", payload, err)
			}
			if strings.Contains(err.Error(), sentinel.Error()) {
				t.Fatal("driver details exposed")
			}
			if kind != "bytes" && state.Closed() != 1 {
				t.Fatalf("cursor closes=%d", state.Closed())
			}
		})
	}
}
func TestBorrowedRowsAreCallerOwned(t *testing.T) {
	plan := testPlan(t)
	db, state := testsql.New(t, sourceStep(plan))
	rows, err := testRows(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if state.Closed() != 0 {
		t.Fatal("native cursor closed before caller iteration")
	}
	if !rows.Next() {
		t.Fatalf("native cursor unavailable: %v", rows.Err())
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if state.Closed() != 1 {
		t.Fatalf("closes=%d", state.Closed())
	}
}

func testPlan(t *testing.T) selection {
	t.Helper()
	plan, err := groupsPlan(19, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
func testRows(ctx context.Context, q rowset.Queryer) (*sql.Rows, error) {
	return GroupsRows(ctx, q, 19, Filter{})
}
func testJSON(ctx context.Context, q rowset.Queryer, limits rowset.Limits) (json.RawMessage, error) {
	return GroupsJSON(ctx, q, 19, Filter{}, limits)
}
