package memory_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const pgaSQL = "SELECT p.VALUE AS BYTES, p.UNIT AS UNIT, p.CON_ID AS CON_ID, " +
	"SYS_CONTEXT('USERENV','CON_ID') AS SESSION_CON_ID, SYS_CONTEXT('USERENV','INSTANCE') AS INST_ID, " +
	"SYS_CONTEXT('USERENV','INSTANCE_NAME',4000) AS INSTANCE_NAME FROM V$PGASTAT p WHERE p.NAME = 'total PGA allocated'"

var pgaColumns = []string{"BYTES", "UNIT", "CON_ID", "SESSION_CON_ID", "INST_ID", "INSTANCE_NAME"}

func TestPGAAllocationReleaseSelectors(t *testing.T) {
	for _, version := range []string{
		"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.0.0.0.0", "26.1.0.0.0", "26ai",
	} {
		got, args, err := memory.PGAAllocationStatement(memory.PGAAllocationOptions{OracleVersion: version})
		if got != "" || args != nil || !errors.Is(err, memory.ErrUnsupportedVersion) {
			t.Fatalf("unsupported %s selected %q %v: %v", version, got, args, err)
		}
	}
	for _, version := range []string{
		"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.4.0.0.0", "19.3.0.0.0", "19.33.0.1.0",
		"21.3.0.0.0", "23.2.0.0.0", "23.9.0.0.0", "23.26.1.0.0", "23.26.4.0.0",
	} {
		got, args, err := memory.PGAAllocationStatement(memory.PGAAllocationOptions{OracleVersion: version})
		if got != pgaSQL || len(args) != 0 || err != nil {
			t.Fatalf("version %s selected %q %v: %v", version, got, args, err)
		}
	}
	const oldSQL = "SELECT TO_CHAR(ROUND(VALUE/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$PGASTAT WHERE NAME='total PGA allocated'"
	got, args, err := memory.PGAAllocatedStatement(memory.PGAAllocatedOptions{OracleMajor: 19})
	if err != nil || got != oldSQL || len(args) != 0 {
		t.Fatalf("rounded API changed SQL: %q %v", got, err)
	}
}

func pgaValues(bytes session.Value, sessionContainer string) []session.Value {
	return []session.Value{bytes, {Text: "bytes"}, {Text: "0"}, {Text: sessionContainer}, {Text: "1"}, {Text: "node1"}}
}

func TestDecodePGAAllocationValidatesScopeAndKeepsExactFields(t *testing.T) {
	opts := memory.PGAAllocationOptions{OracleVersion: "19.33.0.1.0"}
	for _, row := range [][]session.Value{
		pgaValues(session.Value{Text: "9007199254740993"}, "62"),
		pgaValues(session.Value{Null: true}, "1"),
		pgaValues(session.Value{Text: "0"}, "0"),
	} {
		got, err := memory.DecodePGAAllocation(session.Result{Columns: pgaColumns, Rows: [][]session.Value{row}}, opts)
		want := memory.PGAAllocationRow{
			Bytes: row[0], Unit: row[1], ContainerID: row[2], SessionContainerID: row[3], InstanceID: row[4], InstanceName: row[5],
		}
		if err != nil || got.Provenance.Complete || got.Provenance.Source != "V$PGASTAT" ||
			got.Provenance.Scope != "executing_instance_pga_allocation" || !slices.Equal(got.Rows, []memory.PGAAllocationRow{want}) {
			t.Fatalf("unexpected fact %+v: %v", got, err)
		}
	}
	empty, err := memory.DecodePGAAllocation(session.Result{Columns: pgaColumns}, opts)
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("source empty not preserved: %+v %v", empty, err)
	}

	for _, bad := range []struct {
		name  string
		field int
		value session.Value
	}{
		{"negative bytes", 0, session.Value{Text: "-1"}},
		{"fractional bytes", 0, session.Value{Text: "1.5"}},
		{"unknown unit", 1, session.Value{Text: "percent"}},
		{"null unit", 1, session.Value{Null: true}},
		{"opposing scope", 2, session.Value{Text: "62"}},
		{"unknown source scope", 2, session.Value{Null: true}},
		{"unknown session scope", 3, session.Value{Null: true}},
		{"missing member", 4, session.Value{Text: "0"}},
		{"missing instance name", 5, session.Value{Null: true}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			row := pgaValues(session.Value{Text: "0"}, "62")
			row[bad.field] = bad.value
			got, err := memory.DecodePGAAllocation(session.Result{Columns: pgaColumns, Rows: [][]session.Value{row}}, opts)
			if !errors.Is(err, memory.ErrInvalidObservedEvidence) || got.Rows != nil {
				t.Fatalf("unqualified scope returned %+v %v", got, err)
			}
		})
	}
	row := pgaValues(session.Value{Text: "1"}, "62")
	if got, err := memory.DecodePGAAllocation(session.Result{Columns: pgaColumns, Rows: [][]session.Value{row, row}}, opts); !errors.Is(err, memory.ErrInvalidObservedEvidence) || got.Rows != nil {
		t.Fatalf("duplicate samples yielded %+v: %v", got, err)
	}
}

type pgaReader func(context.Context, session.Statement) (session.Result, error)

func (fn pgaReader) Query(ctx context.Context, stmt session.Statement) (session.Result, error) {
	return fn(ctx, stmt)
}

func TestReadPGAAllocationRequiresCompleteBoundedReader(t *testing.T) {
	ref, err := reference.NewReference("sdk", "native", "pga_allocation", 1)
	if err != nil {
		t.Fatal(err)
	}
	transport := errors.New("private read failure")
	for _, tc := range []struct {
		name   string
		limit  rowset.Limits
		err    error
		cancel bool
		want   error
	}{
		{"complete", rowset.Limits{}, nil, false, nil},
		{"capacity", rowset.Limits{MaxRows: 1, MaxBytes: 2}, nil, false, rowset.ErrCapacity},
		{"terminal", rowset.Limits{}, transport, false, transport},
		{"canceled after read", rowset.Limits{}, nil, true, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			reader := pgaReader(func(inner context.Context, stmt session.Statement) (session.Result, error) {
				calls++
				if inner != ctx || stmt.Reference != ref || stmt.Text != pgaSQL || len(stmt.Args) != 0 {
					t.Fatal("result not tied to the exact caller reference, context and select")
				}
				if tc.cancel {
					cancel()
				}
				return session.Result{Columns: pgaColumns, Rows: [][]session.Value{pgaValues(session.Value{Text: "8192"}, "62")}}, tc.err
			})
			got, err := memory.ReadPGAAllocation(ctx, reader, memory.PGAAllocationRequest{
				Options: memory.PGAAllocationOptions{OracleVersion: "19.33.0.1.0"}, Reference: ref, Limits: tc.limit,
			})
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("read error %v, expected %v; calls=%d", err, tc.want, calls)
			}
			if tc.want == nil {
				if !got.Provenance.Complete || len(got.Rows) != 1 || got.Rows[0].SessionContainerID.Text != "62" {
					t.Fatalf("incomplete result %+v", got)
				}
			} else if got.Rows != nil || got.Provenance.Complete {
				t.Fatalf("failed read exposed evidence: %+v", got)
			}
		})
	}
}

func TestPGAAllocationJSONRetainsExactNumbersAndCursorFinalization(t *testing.T) {
	opts := memory.PGAAllocationOptions{OracleVersion: "19.33.0.1.0"}
	step := testsql.Step{
		SQL: pgaSQL, Columns: pgaColumns,
		Values: [][]driver.Value{{"9007199254740993", "bytes", int64(0), "62", "1", "node1"}},
	}
	db, state := testsql.New(t, step)
	payload, err := memory.PGAAllocationJSON(t.Context(), db, opts, rowset.Limits{})
	var envelope struct {
		Provenance memory.DatasetProvenance
		Data       struct{ Rows [][]json.RawMessage }
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 6 ||
		string(envelope.Data.Rows[0][0]) != "9007199254740993" || string(envelope.Data.Rows[0][2]) != "0" ||
		string(envelope.Data.Rows[0][3]) != "62" || state.Closed() != 1 {
		t.Fatalf("JSON lost values or cursor finalization: %s %v", payload, err)
	}
	late := errors.New("terminal cursor")
	step.NextErr = late
	db, state = testsql.New(t, step)
	payload, err = memory.PGAAllocationJSON(t.Context(), db, opts, rowset.Limits{})
	if payload != nil || !errors.Is(err, late) || state.Closed() != 1 {
		t.Fatalf("failed cursor exposed %s: %v", payload, err)
	}
}
