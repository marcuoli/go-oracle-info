package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func sessionClockFixture() session.Result {
	return session.Result{
		Columns: []string{"SYSDATE", "SYSTIMESTAMP_UTC", "CURRENT_DATE", "CURRENT_TIMESTAMP_UTC", "LOCALTIMESTAMP", "SESSION_TIME_ZONE"},
		Rows: [][]session.Value{{
			{Text: "2026-11-25T07:00:00"},
			{Text: "2026-11-25T10:00:00.123456789Z"},
			{Text: "2026-11-25T15:45:00"},
			{Text: "2026-11-25T10:00:00.123456000Z"},
			{Text: "2026-11-25T15:45:00.123456000"},
			{Text: "+05:45"},
		}},
	}
}

func TestSessionClockStatementVersionAndShape(t *testing.T) {
	first, err := SessionClockStatement(SessionClockOptions{OracleVersion: "9.2.0.8.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CAST(SYSDATE AS TIMESTAMP)", "CAST(CURRENT_DATE AS TIMESTAMP)",
		"SYS_EXTRACT_UTC(SYSTIMESTAMP)", "SYS_EXTRACT_UTC(CURRENT_TIMESTAMP)",
		"CAST(LOCALTIMESTAMP AS TIMESTAMP(9))",
		"'FM00D000000000', 'NLS_NUMERIC_CHARACTERS=''.,'''",
		"SESSIONTIMEZONE AS SESSION_TIME_ZONE FROM SYS.DUAL",
	} {
		if !strings.Contains(first, required) {
			t.Errorf("missing clock projection %s", required)
		}
	}
	if strings.Count(first, " || 'Z' END") != 2 || strings.Count(first, "FROM SYS.DUAL") != 1 ||
		strings.Contains(first, "DBTIMEZONE") {
		t.Fatal("wrong UTC labels, time zone source, or statement count")
	}
	for _, version := range []string{
		"10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0",
		"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.4.0.0.0",
		"19.32.0.0.0", "21.21.0.0.0", "23.9.0.0.0", "23.26.1.0.0", "23.26.4.0.0",
	} {
		statement, err := SessionClockStatement(SessionClockOptions{OracleVersion: version})
		if err != nil || statement != first {
			t.Fatalf("version %s chose different SQL: %v", version, err)
		}
	}
	for _, version := range []string{"", "9.1.0", "10.3.0", "26ai", "26.1.0.0.0"} {
		_, err := SessionClockStatement(SessionClockOptions{OracleVersion: version})
		if !errors.Is(err, ErrUnsupportedVersion) {
			t.Errorf("admitted unsupported version %q: %v", version, err)
		}
	}
}

func TestDecodeSessionClockPreservesCivilUTCNullAndEmpty(t *testing.T) {
	options := SessionClockOptions{OracleVersion: "23.26.1.0.0"}
	fixture := sessionClockFixture()
	v := fixture.Rows[0]
	// Reorder metadata and retain a projected NULL independently of civil time.
	fixture.Columns[0], fixture.Columns[5] = fixture.Columns[5], fixture.Columns[0]
	v[0], v[5] = session.Value{Text: "Asia/Kathmandu"}, v[0]
	v[2] = session.Value{Null: true}
	result, err := DecodeSessionClock(fixture, options)
	want := SessionClockRow{
		SysDate: v[5], SysTimestampUTC: v[1], CurrentDate: v[2], CurrentTimestampUTC: v[3],
		LocalTimestamp: v[4], SessionTimeZone: v[0],
	}
	if err != nil || result.Provenance.Complete || result.Provenance.Scope != "connected_session_clock" ||
		!slices.Equal(result.Rows, []SessionClockRow{want}) {
		t.Fatalf("altered values or provenance: %+v, %v", result, err)
	}
	fixture.Rows = nil
	result, err = DecodeSessionClock(fixture, options)
	if err != nil || result.Rows == nil || len(result.Rows) != 0 {
		t.Fatalf("empty source result: %+v, %v", result, err)
	}
	for _, entry := range []struct {
		index int
		text  string
	}{
		{0, "2026-11-25T07:00:00.0"},
		{1, "2026-11-25T10:00:00.123456789+00:00"},
		{2, "2026-02-31T15:45:00"},
		{3, "2026-11-25T10:00:00.123456Z"},
		{4, "2026-11-25T15:45:00,123456000"},
	} {
		fixture := sessionClockFixture()
		fixture.Rows[0][entry.index].Text = entry.text
		result, err := DecodeSessionClock(fixture, options)
		if !errors.Is(err, ErrInvalidEvidence) || result.Rows != nil {
			t.Errorf("malformed column %d admitted: %+v, %v", entry.index, result, err)
		}
	}
}

func TestReadSessionClockOneBoundedQuery(t *testing.T) {
	options := SessionClockOptions{OracleVersion: "19.32.0.0.0"}
	request := SessionClockRequest{Options: options, Reference: coreReference(t)}
	source := sessionClockFixture()
	sql, err := SessionClockStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	reader := datasetTestReader(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
		calls++
		if ctx != t.Context() || stmt.Text != sql || stmt.Reference != request.Reference || len(stmt.Args) != 0 {
			t.Fatalf("unexpected request: %+v", stmt)
		}
		return source, nil
	})
	facts, err := ReadSessionClock(t.Context(), reader, request)
	if err != nil || calls != 1 || !facts.Provenance.Complete || len(facts.Rows) != 1 ||
		facts.Rows[0].SysTimestampUTC != source.Rows[0][1] {
		t.Fatalf("single observation returned %+v, %v, calls=%d", facts, err, calls)
	}

	request.Limits = rowset.Limits{MaxRows: 1, MaxBytes: 5}
	facts, err = ReadSessionClock(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || facts.Rows != nil {
		t.Fatalf("capacity returned %+v, %v", facts, err)
	}

	request.Limits = rowset.Limits{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	facts, err = ReadSessionClock(ctx, datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return source, nil
	}), request)
	if !errors.Is(err, context.Canceled) || facts.Rows != nil {
		t.Fatalf("cancellation returned %+v, %v", facts, err)
	}
}

func TestSessionClockJSONFinalizesAndValidatesBeforePublishing(t *testing.T) {
	options := SessionClockOptions{OracleVersion: "23.26.1.0.0"}
	statement, err := SessionClockStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	fixture := sessionClockFixture()
	values := make([]driver.Value, len(fixture.Columns))
	for i, v := range fixture.Rows[0] {
		values[i] = v.Text
	}
	values[4] = nil
	step := testsql.Step{SQL: statement, Columns: fixture.Columns, Values: [][]driver.Value{values}}
	db, state := testsql.New(t, step)
	payload, err := SessionClockJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance DatasetProvenance `json:"provenance"`
		Data       struct {
			Rows [][]json.RawMessage `json:"rows"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 6 ||
		string(envelope.Data.Rows[0][1]) != "\"2026-11-25T10:00:00.123456789Z\"" ||
		string(envelope.Data.Rows[0][4]) != "null" || state.Closed() != 1 {
		t.Fatalf("unexpected bounded JSON %s, %v (closes=%d)", payload, err, state.Closed())
	}
	step.Values = append(step.Values, values)
	db, state = testsql.New(t, step)
	payload, err = SessionClockJSON(t.Context(), db, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatalf("extra row returned %s, %v (closes=%d)", payload, err, state.Closed())
	}
}
