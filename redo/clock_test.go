package redo

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func clockSource(major int) session.Result {
	result := session.Result{Columns: []string{"INST_ID", "LOCAL_TIME"}, Rows: [][]session.Value{{{Text: "2"}, {Text: "2026-11-15T23:59:58"}}}}
	if major >= 12 {
		result.Columns = append(result.Columns, "CON_ID")
		result.Rows[0] = append(result.Rows[0], session.Value{Null: true})
	}
	return result
}
func clockStep(major int) testsql.Step {
	statement, _ := ClockStatement(major)
	result := clockSource(major)
	values := make([]driver.Value, len(result.Columns))
	for i, v := range result.Rows[0] {
		if !v.Null {
			values[i] = v.Text
		}
	}
	return testsql.Step{SQL: statement, Columns: result.Columns, Values: [][]driver.Value{values}}
}
func TestClockExactCivilLabelAndVersionScope(t *testing.T) {
	for _, major := range []int{11, 19, 23} {
		t.Run(strconv.Itoa(major), func(t *testing.T) {
			source := clockSource(major)
			clock, err := DecodeClock(source, major)
			if err != nil {
				t.Fatal(err)
			}
			if clock.LocalTime.Text != "2026-11-15T23:59:58" || clock.ReportingInstanceID.Text != "2" || clock.Provenance.Complete || clock.Provenance.Source != "SYSDATE" || clock.Provenance.Scope != "connected_instance_clock" {
				t.Fatalf("changed source clock meaning: %+v", clock)
			}
			if major == 11 && clock.ContainerID != nil {
				t.Fatal("invented 11g container")
			}
			if major >= 12 && (clock.ContainerID == nil || !clock.ContainerID.Null) {
				t.Fatal("lost projected container NULL")
			}
			statement, err := ClockStatement(major)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(statement, "EXTRACT(YEAR FROM CAST(SYSDATE AS TIMESTAMP))") || !strings.HasSuffix(statement, " FROM SYS.DUAL") || strings.Contains(statement, "SYSTIMESTAMP") || strings.Contains(statement, "CURRENT_TIMESTAMP") || strings.Contains(statement, "TO_CHAR(SYSDATE,") {
				t.Fatalf("calendar/zone semantics changed: %s", statement)
			}
			if (major == 11) == strings.Contains(statement, "CON_ID") {
				t.Fatal("wrong optional container projection")
			}
		})
	}
	if _, err := ClockStatement(10); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("unsupported clock source admitted: %v", err)
	}
}
func TestClockDecodeRejectsAbsentNullMalformedAndMultipleRows(t *testing.T) {
	for _, bad := range []string{"", "2026-02-30T12:00:00", "2026-11-15T23:59:58Z", "2026-11-15T23:59:58+00:00", "2026-11-15T23:59:58.001", "0000-01-01T00:00:00"} {
		source := clockSource(19)
		source.Rows[0][1] = session.Value{Text: bad}
		clock, err := DecodeClock(source, 19)
		if !errors.Is(err, ErrInvalidEvidence) || clock.LocalTime.Text != "" || clock.Provenance.Complete {
			t.Fatalf("accepted malformed clock %q: %+v %v", bad, clock, err)
		}
	}
	for _, kind := range []string{"no-rows", "null-clock", "multiple", "bad-instance", "null-instance", "wrong-schema"} {
		t.Run(kind, func(t *testing.T) {
			source := clockSource(19)
			switch kind {
			case "no-rows":
				source.Rows = nil
			case "null-clock":
				source.Rows[0][1] = session.Value{Null: true}
			case "multiple":
				source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
			case "bad-instance":
				source.Rows[0][0] = session.Value{Text: "0"}
			case "null-instance":
				source.Rows[0][0] = session.Value{Null: true}
			case "wrong-schema":
				source.Columns[1] = "UTC_TIME"
			}
			clock, err := DecodeClock(source, 19)
			if !errors.Is(err, ErrInvalidEvidence) || clock.Provenance.Complete || clock.LocalTime.Text != "" {
				t.Fatalf("accepted invalid clock source: %+v %v", clock, err)
			}
		})
	}
}
func TestClockReaderAdmissionCompletenessAndCancellation(t *testing.T) {
	reference := sourceRef(t)
	source := clockSource(19)
	reader := &fixtureReader{result: source}
	request := Request{OracleMajor: 19, Reference: reference}
	clock, err := ReadClock(context.Background(), reader, request)
	if err != nil || !clock.Provenance.Complete || len(reader.calls) != 1 || reader.calls[0].Reference != reference || len(reader.calls[0].Args) != 0 {
		t.Fatalf("clock reader contract: %+v %v", clock, err)
	}
	reader = &fixtureReader{result: source}
	request.Filter.Instances = []int64{2}
	clock, err = ReadClock(context.Background(), reader, request)
	if !errors.Is(err, ErrInvalidRequest) || len(reader.calls) != 0 || clock.Provenance.Complete {
		t.Fatal("ignored nonlocal instance admission")
	}
	request.Filter = Filter{}
	failure := errors.New("private clock query failure")
	clock, err = ReadClock(context.Background(), &fixtureReader{result: source, err: failure}, request)
	if !errors.Is(err, failure) || clock.Provenance.Complete || clock.LocalTime.Text != "" {
		t.Fatalf("query prefix published: %+v %v", clock, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	clock, err = ReadClock(ctx, &fixtureReader{result: source, after: cancel}, request)
	if !errors.Is(err, context.Canceled) || clock.Provenance.Complete || clock.LocalTime.Text != "" {
		t.Fatalf("cancelled clock published: %+v %v", clock, err)
	}
}
func TestClockJSONExactNullAndAtomicDomainFailures(t *testing.T) {
	db, state := testsql.New(t, clockStep(19))
	payload, err := ClockJSON(context.Background(), db, 19, rowset.Limits{MaxRows: 1, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance Provenance
		Data       struct{ Rows [][]any }
	}
	if err = json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.Complete || len(envelope.Data.Rows) != 1 || envelope.Data.Rows[0][1] != "2026-11-15T23:59:58" || envelope.Data.Rows[0][2] != nil || state.Closed() != 1 {
		t.Fatalf("lost exact source civil label/NULL: %s", payload)
	}
	failure := errors.New("private terminal detail")
	for _, kind := range []string{"no-rows", "null", "zone", "late-row", "next", "close", "cancel", "row-budget", "byte-budget"} {
		t.Run(kind, func(t *testing.T) {
			step := clockStep(19)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := rowset.Limits{MaxRows: 2, MaxBytes: 4096}
			switch kind {
			case "no-rows":
				step.Values = nil
			case "null":
				step.Values[0][1] = nil
			case "zone":
				step.Values[0][1] = "2026-11-15T23:59:58Z"
			case "late-row":
				step.Values = append(step.Values, append([]driver.Value(nil), step.Values[0]...))
			case "next":
				step.NextErr = failure
			case "close":
				step.CloseErr = failure
			case "cancel":
				step.OnNext = func(i int) {
					if i == 1 {
						cancel()
					}
				}
			case "row-budget":
				step.Values = append(step.Values, step.Values[0])
				limits.MaxRows = 1
			case "byte-budget":
				limits.MaxBytes = 16
			}
			db, state := testsql.New(t, step)
			payload, err := ClockJSON(ctx, db, 19, limits)
			if err == nil || payload != nil || strings.Contains(err.Error(), failure.Error()) {
				t.Fatalf("failed clock JSON published/leaked: %s %v", payload, err)
			}
			if kind != "byte-budget" && state.Closed() != 1 {
				t.Fatalf("closes=%d", state.Closed())
			}
			if kind == "late-row" && !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("late domain admission bypassed: %v", err)
			}
		})
	}
}
func TestClockNativeRowsRemainCallerOwned(t *testing.T) {
	db, state := testsql.New(t, clockStep(19))
	rows, err := ClockRows(context.Background(), db, 19)
	if err != nil {
		t.Fatal(err)
	}
	if state.Closed() != 0 || !rows.Next() {
		t.Fatalf("cursor unavailable early: %v", rows.Err())
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if state.Closed() != 1 {
		t.Fatal("native cursor not finalized by caller")
	}
}
