package storage

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestTempFreeSpaceReleaseProjection(t *testing.T) {
	for _, tc := range []struct {
		version     string
		container   bool
		owner       bool
		unsupported bool
	}{
		{version: "9.2.0.8.0", unsupported: true},
		{version: "10.1.0.5.0", unsupported: true},
		{version: "10.2.0.5.0", unsupported: true},
		{version: "11.1.0.7.0"},
		{version: "11.2.0.4.0"},
		{version: "12.1.0.1.0", container: true},
		{version: "12.1.0.2.0", container: true},
		{version: "12.2.0.0.0", unsupported: true},
		{version: "12.2", unsupported: true},
		{version: "12.2.0.1.0", container: true, owner: true},
		{version: "18.1.0.0.0", container: true, owner: true},
		{version: "19.32.0.0.0", container: true, owner: true},
		{version: "21.3.0.0.0", container: true, owner: true},
		{version: "23.0.0.0.0", container: true, owner: true},
		{version: "23.26.1.0.0", container: true, owner: true},
		{version: "26.1.0.0.0", unsupported: true},
		{version: "19.x.0.0.0", unsupported: true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			sql, err := TempFreeSpaceStatement(tc.version)
			if tc.unsupported {
				reader := &fixtureReader{}
				got, readErr := ReadTempFreeSpace(t.Context(), reader, TempFreeSpaceRequest{OracleVersion: tc.version, Reference: sourceRef(t)})
				if !errors.Is(err, ErrUnsupportedVersion) || sql != "" || !errors.Is(readErr, ErrUnsupportedVersion) || len(reader.calls) != 0 || got.Rows != nil {
					t.Fatalf("unsupported selection executed: %q %v %+v %v", sql, err, got, readErr)
				}
				return
			}
			if err != nil || !strings.Contains(sql, " FROM DBA_TEMP_FREE_SPACE v ") ||
				!strings.Contains(sql, "TO_CHAR(v.FREE_SPACE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS FREE_SPACE") ||
				!strings.Contains(sql, "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID") ||
				strings.Contains(sql, " AS SOURCE_CON_ID") != tc.container ||
				strings.Contains(sql, "v.SHARED AS SHARED") != tc.owner ||
				strings.Contains(sql, "v.INST_ID AS OWNING_INST_ID") != tc.owner {
				t.Fatalf("wrong release projection: %q %v", sql, err)
			}
		})
	}
}

func tempSpaceResult() session.Result {
	return session.Result{Columns: []string{"TABLESPACE_NAME", "TABLESPACE_SIZE", "ALLOCATED_SPACE", "FREE_SPACE", "SOURCE_INST_ID", "SOURCE_CON_ID", "SHARED", "OWNING_INST_ID"}, Rows: [][]session.Value{
		{{Text: "TEMP"}, {Text: "100000000000000000000"}, {Text: "1200"}, {Text: "99999999999999999998"}, {Text: "1"}, {Text: "3"}, {Text: "SHARED"}, {Null: true}},
		{{Text: "TEMP"}, {Text: "90"}, {Text: "80"}, {Null: true}, {Text: "1"}, {Text: "3"}, {Text: "LOCAL_FOR_ALL"}, {Text: "2"}},
		{{Text: "TEMP"}, {Text: "90"}, {Text: "90"}, {Text: "0"}, {Text: "1"}, {Text: "3"}, {Text: "LOCAL_FOR_ALL"}, {Text: "3"}},
	}}
}

func TestTempFreeSpaceTypedEvidence(t *testing.T) {
	const version = "23.26.1.0.0"
	source := tempSpaceResult()
	facts, err := DecodeTempFreeSpace(source, version)
	if err != nil || len(facts.Rows) != 3 || facts.Provenance.Complete || facts.Provenance.Scope != "current_container_dictionary" {
		t.Fatalf("wrong decoded evidence: %+v %v", facts, err)
	}
	shared, local, full := facts.Rows[0], facts.Rows[1], facts.Rows[2]
	if shared.FreeSpace.Text != "99999999999999999998" || shared.FreeSpace.Null ||
		shared.AllocatedSpace.Text != "1200" || shared.Shared.Text != "SHARED" || !shared.OwningInstanceID.Null ||
		shared.SourceContainerID.Text != "3" || local.ReportingInstanceID.Text != "1" ||
		local.OwningInstanceID.Text != "2" || local.TablespaceName != shared.TablespaceName ||
		!local.FreeSpace.Null || full.FreeSpace.Null || full.FreeSpace.Text != "0" || full.OwningInstanceID.Text != "3" {
		t.Fatalf("lost exact, duplicate, null or scope evidence: %+v", facts)
	}
	source.Columns, source.Rows = source.Columns[:5], nil
	old, err := DecodeTempFreeSpace(session.Result{Columns: source.Columns, Rows: [][]session.Value{tempSpaceResult().Rows[0][:5]}}, "11.2.0.4.0")
	if err != nil || len(old.Rows) != 1 || old.Rows[0].SourceContainerID != nil || old.Rows[0].Shared != nil || old.Rows[0].OwningInstanceID != nil {
		t.Fatalf("fabricated legacy fields: %+v %v", old, err)
	}
	got, err := ReadTempFreeSpace(t.Context(), &fixtureReader{result: source}, TempFreeSpaceRequest{OracleVersion: "11.2.0.4.0", Reference: sourceRef(t)})
	if err != nil || !got.Provenance.Complete || got.Rows == nil || len(got.Rows) != 0 {
		t.Fatalf("empty selection was not distinct: %+v %v", got, err)
	}
}

func TestTempFreeSpaceRejectsLateFailures(t *testing.T) {
	req := TempFreeSpaceRequest{OracleVersion: "19.32.0.0.0", Reference: sourceRef(t)}
	for _, tc := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"null-name", func(r *session.Result) { r.Rows[2][0] = session.Value{Null: true} }},
		{"negative-byte", func(r *session.Result) { r.Rows[2][3].Text = "-1" }},
		{"decimal-byte", func(r *session.Result) { r.Rows[2][3].Text = "1.5" }},
		{"contradictory-null", func(r *session.Result) { r.Rows[2][3].Null = true }},
		{"late-width", func(r *session.Result) { r.Rows[2] = r.Rows[2][:7] }},
		{"owner-as-text", func(r *session.Result) { r.Rows[2][7].Text = "owner" }},
		{"missing-reporter", func(r *session.Result) { r.Rows[2][4] = session.Value{Null: true} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tempSpaceResult()
			tc.change(&source)
			got, err := ReadTempFreeSpace(t.Context(), &fixtureReader{result: source}, req)
			if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil || got.Provenance.Complete {
				t.Fatalf("published malformed late evidence: %+v %v", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, tc := range []struct {
		ctx   context.Context
		r     *fixtureReader
		cause error
	}{
		{t.Context(), &fixtureReader{result: tempSpaceResult(), err: io.ErrUnexpectedEOF}, io.ErrUnexpectedEOF},
		{ctx, &fixtureReader{result: tempSpaceResult(), after: cancel}, context.Canceled},
	} {
		got, err := ReadTempFreeSpace(tc.ctx, tc.r, req)
		if !errors.Is(err, tc.cause) || got.Rows != nil || got.Provenance.Complete {
			t.Fatalf("failed reader returned a valid prefix: %+v %v", got, err)
		}
	}
}

func TestTempFreeSpaceJSONExactValuesAndAtomicFailure(t *testing.T) {
	const version = "19.32.0.0.0"
	statement, err := TempFreeSpaceStatement(version)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{SQL: statement, Columns: tempSpaceResult().Columns, Values: [][]driver.Value{
		{"TEMP", "100000000000000000000", "1200", "99999999999999999998", int64(1), int64(3), "SHARED", nil},
		{"TEMP", "90", "80", nil, int64(1), int64(3), "LOCAL_FOR_ALL", int64(2)},
	}}
	limits := rowset.Limits{MaxRows: 2, MaxBytes: 8192}
	db, state := testsql.New(t, step)
	payload, err := TempFreeSpaceJSON(t.Context(), db, version, limits)
	var envelope struct {
		Provenance Provenance
		Data       struct{ Rows [][]any }
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		state.Closed() != 1 || len(envelope.Data.Rows) != 2 {
		t.Fatalf("incomplete JSON: %s %v", payload, err)
	}
	rows := envelope.Data.Rows
	if rows[0][3] != "99999999999999999998" || rows[0][7] != nil || rows[1][3] != nil ||
		rows[1][4] != float64(1) || rows[1][5] != float64(3) || rows[1][7] != float64(2) {
		t.Fatalf("JSON lost exact text, NULL or scope: %s", payload)
	}
	for _, tc := range []struct {
		name   string
		change func(*testsql.Step, *rowset.Limits)
		cause  error
	}{
		{"bad-late-row", func(s *testsql.Step, _ *rowset.Limits) { s.Values[1][3] = "1.5" }, ErrInvalidEvidence},
		{"close-error", func(s *testsql.Step, _ *rowset.Limits) { s.CloseErr = io.ErrUnexpectedEOF }, io.ErrUnexpectedEOF},
		{"row-budget", func(_ *testsql.Step, l *rowset.Limits) { l.MaxRows = 1 }, rowset.ErrCapacity},
		{"envelope-budget", func(_ *testsql.Step, l *rowset.Limits) { l.MaxBytes = len(payload) - 1 }, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, l := step, limits
			s.Values = [][]driver.Value{append([]driver.Value(nil), step.Values[0]...), append([]driver.Value(nil), step.Values[1]...)}
			tc.change(&s, &l)
			db, state := testsql.New(t, s)
			data, err := TempFreeSpaceJSON(t.Context(), db, version, l)
			if !errors.Is(err, tc.cause) || data != nil || state.Closed() != 1 {
				t.Fatalf("published failed dataset: %s %v", data, err)
			}
		})
	}
}
