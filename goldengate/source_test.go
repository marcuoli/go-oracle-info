package goldengate

import (
	"context"
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

type readerFixture struct {
	result session.Result
	err    error
	after  func()
	calls  int
}

func (r *readerFixture) Query(context.Context, session.Statement) (session.Result, error) {
	r.calls++
	if r.after != nil {
		r.after()
	}
	return r.result, r.err
}
func ref(t *testing.T) reference.Reference {
	t.Helper()
	r, e := reference.NewReference("unit", "facts", "goldengate", 1)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func fixture(p selection) session.Result {
	result := session.Result{}
	for _, c := range p.schema {
		result.Columns = append(result.Columns, c.Name)
	}
	for _, id := range []string{"1", "2"} {
		values := make([]session.Value, len(p.schema))
		for i, c := range p.schema {
			v := "0"
			switch c.Name {
			case "INST_ID":
				v = id
			case "CON_ID":
				v = "3"
			case "CAPTURE_NAME", "APPLY_NAME":
				v = "same_process"
			case "STATE":
				v = "IDLE"
			case "LWM_MESSAGE_NUMBER", "CAPTURE_MESSAGE_NUMBER":
				v = "9007199254740993123456789"
			case "TOTAL_MESSAGES_CAPTURED", "TOTAL_APPLIED":
				v = "9007199254740993"
			}
			values[i] = session.Value{Text: v}
		}
		result.Rows = append(result.Rows, values)
	}
	return result
}
func processPlanForTest(version string, apply bool) (selection, error) {
	if apply {
		return applyCoordinatorsPlan(version)
	}
	return capturePlan(version)
}

func TestVersionProjectionScopeAndExactRows(t *testing.T) {
	for _, v := range []string{"11.2.0.4.0", "12.1.0.2.0", "19.26.0.0.0", "23.26.1.0.0"} {
		for _, apply := range []bool{false, true} {
			p, err := processPlanForTest(v, apply)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(p.sql, "FROM GV$") || strings.Contains(p.sql, "WHERE") || strings.Contains(p.sql, "ROWNUM") {
				t.Fatal(p.sql)
			}
			if strings.Contains(p.sql, "CON_ID") != !strings.HasPrefix(v, "11.") {
				t.Fatal(p.sql)
			}
			r := &readerFixture{result: fixture(p)}
			request := Request{OracleVersion: v, Reference: ref(t)}
			if apply {
				got, e := ReadApplyCoordinators(t.Context(), r, request)
				if e != nil || len(got.Rows) != 2 || got.Rows[1].InstanceID.Text != "2" || got.Rows[0].LowWatermarkMessageNumber.Text != "9007199254740993123456789" || !got.Provenance.Complete {
					t.Fatal(got, e)
				}
			} else {
				r.result.Rows[0][2] = session.Value{Null: true}
				got, e := ReadCapture(t.Context(), r, request)
				if e != nil || len(got.Rows) != 2 || got.Rows[1].InstanceID.Text != "2" || !got.Rows[0].State.Null || got.Rows[0].MessagesCaptured.Text != "9007199254740993" || !got.Provenance.Complete {
					t.Fatal(got, e)
				}
				if (got.Rows[0].ContainerID == nil) != strings.HasPrefix(v, "11.") {
					t.Fatal("optional CON_ID lost")
				}
			}
			if r.calls != 1 {
				t.Fatal(r.calls)
			}
		}
	}
	for _, v := range []string{"", "11", "11.1.0.7.0", "11.2.0.3.0", "19x.0.0.0.0"} {
		for _, apply := range []bool{false, true} {
			if _, e := processPlanForTest(v, apply); !errors.Is(e, ErrUnsupportedVersion) {
				t.Fatal(v, e)
			}
		}
	}
}
func TestFailedOrCancelledReadsNeverPublishPrefix(t *testing.T) {
	for _, mode := range []string{"late-shape", "negative", "bad-null", "zero-instance", "terminal", "cancel", "empty"} {
		t.Run(mode, func(t *testing.T) {
			p, _ := capturePlan("19.26.0.0.0")
			r := &readerFixture{result: fixture(p)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch mode {
			case "late-shape":
				r.result.Rows = append(r.result.Rows, nil)
			case "negative":
				r.result.Rows[1][4].Text = "-1"
			case "bad-null":
				r.result.Rows[1][2] = session.Value{Text: "private", Null: true}
			case "zero-instance":
				r.result.Rows[1][0].Text = "0"
			case "terminal":
				r.err = errors.New("private driver credentials")
			case "cancel":
				r.after = cancel
			case "empty":
				r.result.Rows = nil
			}
			got, e := ReadCapture(ctx, r, Request{OracleVersion: "19.26.0.0.0", Reference: ref(t)})
			if mode == "empty" {
				if e != nil || len(got.Rows) != 0 || !got.Provenance.Complete {
					t.Fatal(got, e)
				}
				return
			}
			if e == nil || got.Rows != nil || got.Provenance.Complete || strings.Contains(e.Error(), "private") {
				t.Fatal(got, e)
			}
			if mode == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}
func nativeStep(p selection) testsql.Step {
	f := fixture(p)
	step := testsql.Step{SQL: p.sql, Columns: f.Columns}
	for _, r := range f.Rows {
		values := make([]driver.Value, len(r))
		for i, v := range r {
			// Pin native driver types to the SQL contract, not the schema under test.
			// Message positions are text because their NUMBER projection uses TO_CHAR.
			values[i] = v.Text
			switch p.schema[i].Name {
			case "INST_ID":
				if v.Text == "1" {
					values[i] = int64(1)
				} else {
					values[i] = int64(2)
				}
			case "CON_ID":
				values[i] = int64(3)
			case "TOTAL_ERRORS":
				values[i] = int64(0)
			case "TOTAL_MESSAGES_CAPTURED", "TOTAL_APPLIED":
				values[i] = int64(9007199254740993)
			}
		}
		step.Values = append(step.Values, values)
	}
	return step
}
func TestJSONAtomicityForBothSources(t *testing.T) {
	for _, apply := range []bool{false, true} {
		for _, mode := range []string{"ok", "next", "close", "rows", "bytes", "late-id", "cancel"} {
			t.Run(mode+map[bool]string{false: "capture", true: "apply"}[apply], func(t *testing.T) {
				p, _ := processPlanForTest("19.26.0.0.0", apply)
				step := nativeStep(p)
				limits := rowset.Limits{}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				sentinel := errors.New("private terminal error")
				switch mode {
				case "next":
					step.NextErr = sentinel
				case "close":
					step.CloseErr = sentinel
				case "rows":
					limits.MaxRows = 1
				case "bytes":
					limits.MaxBytes = 500
				case "late-id":
					step.Values[1][0] = int64(0)
				case "cancel":
					step.OnNext = func(n int) {
						if n == 1 {
							cancel()
						}
					}
				}
				db, state := testsql.New(t, step)
				var payload json.RawMessage
				var err error
				if apply {
					payload, err = ApplyCoordinatorsJSON(ctx, db, "19.26.0.0.0", limits)
				} else {
					payload, err = CaptureJSON(ctx, db, "19.26.0.0.0", limits)
				}
				if mode != "ok" {
					if err == nil || payload != nil {
						t.Fatal("failed prefix", string(payload), err)
					}
				} else {
					if err != nil || !json.Valid(payload) || !strings.Contains(string(payload), "9007199254740993") || !strings.Contains(string(payload), `"9007199254740993123456789"`) {
						t.Fatal(string(payload), err)
					}
				}
				if state.Closed() != 1 || len(state.Calls()) != 1 {
					t.Fatal("lifecycle changed", state.Closed(), state.Calls())
				}
			})
		}
	}
}

func TestMessageNumbersUseExplicitTextProjection(t *testing.T) {
	for _, version := range []string{"11.2.0.4.0", "12.1.0.2.0", "19.26.0.0.0", "23.26.1.0.0"} {
		for _, apply := range []bool{false, true} {
			name, field := "capture", "CAPTURE_MESSAGE_NUMBER"
			if apply {
				name, field = "apply", "LWM_MESSAGE_NUMBER"
			}
			t.Run(version+"/"+name, func(t *testing.T) {
				var statement string
				var err error
				if apply {
					statement, err = ApplyCoordinatorsStatement(version)
				} else {
					statement, err = CaptureStatement(version)
				}
				want := "TO_CHAR(v." + field + ",'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS " + field
				if err != nil || !strings.Contains(statement, want) {
					t.Fatalf("missing exact text projection %q in %q: %v", want, statement, err)
				}
			})
		}
	}
}

func TestMessageNumberJSONPreservesTextAndNull(t *testing.T) {
	for _, apply := range []bool{false, true} {
		name, field := "capture", "CAPTURE_MESSAGE_NUMBER"
		if apply {
			name, field = "apply", "LWM_MESSAGE_NUMBER"
		}
		for _, tc := range []struct {
			name  string
			value driver.Value
			want  string
		}{
			{"zero", "0", `"0"`}, {"integer", "42", `"42"`},
			{"beyond_int64", "9007199254740993123456789", `"9007199254740993123456789"`},
			{"null", nil, "null"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				p, err := processPlanForTest("19.26.0.0.0", apply)
				if err != nil {
					t.Fatal(err)
				}
				step := nativeStep(p)
				for i, column := range step.Columns {
					if column == field {
						step.Values[0][i] = tc.value
					}
				}
				db, state := testsql.New(t, step)
				var payload json.RawMessage
				if apply {
					payload, err = ApplyCoordinatorsJSON(t.Context(), db, "19.26.0.0.0", rowset.Limits{})
				} else {
					payload, err = CaptureJSON(t.Context(), db, "19.26.0.0.0", rowset.Limits{})
				}
				if err != nil {
					t.Fatal(err)
				}
				var envelope struct {
					Data struct {
						Columns []rowset.Column     `json:"columns"`
						Rows    [][]json.RawMessage `json:"rows"`
					} `json:"data"`
				}
				if err := json.Unmarshal(payload, &envelope); err != nil {
					t.Fatal(err)
				}
				if len(envelope.Data.Rows) != 2 {
					t.Fatalf("lost member rows: %s", payload)
				}
				found := false
				for i, column := range envelope.Data.Columns {
					if column.Name == field {
						found = true
						if column.Kind != rowset.Text || string(envelope.Data.Rows[0][i]) != tc.want {
							t.Fatalf("message number kind/value changed: %s", payload)
						}
					}
				}
				if !found || state.Closed() != 1 {
					t.Fatal("message column or cursor finalization lost")
				}
			})
		}
	}
}
