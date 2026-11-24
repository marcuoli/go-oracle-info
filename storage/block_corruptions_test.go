package storage

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

var corruptionColumns = []string{"FILE_ID", "BLOCK_ID", "BLOCKS", "CORRUPTION_CHANGE", "CORRUPTION_TYPE", "CON_ID"}

func TestBlockCorruptionsReleaseProjection(t *testing.T) {
	for _, tc := range []struct {
		version string
		modern  bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false},
		{"12.1.0.1.0", true}, {"12.1.0.2.0", true}, {"12.2.0.1.0", true},
		{"18.20.0.0.0", true}, {"19.32.0.0.0", true}, {"21.24.0.0.0", true},
		{"23.4.0.0.0", true}, {"23.26.1.0.0", true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			sql, err := BlockCorruptionsStatement(tc.version)
			if err != nil || !strings.Contains(sql, " FROM V$DATABASE_BLOCK_CORRUPTION c ORDER BY ") {
				t.Fatalf("incorrect source: %q, %v", sql, err)
			}
			if strings.Contains(sql, "c.CON_ID AS CON_ID") != tc.modern {
				t.Fatalf("incorrect container projection: %q", sql)
			}
			for _, expr := range []string{"c.FILE#", "c.BLOCK#", "c.BLOCKS", "c.CORRUPTION_CHANGE#", "c.CORRUPTION_TYPE"} {
				if !strings.Contains(sql, expr) {
					t.Errorf("missing Oracle field %s", expr)
				}
			}
		})
	}
	for _, version := range []string{"", "9.0.1", "12.0", "26ai", "27.1.0.0.0"} {
		if sql, err := BlockCorruptionsStatement(version); sql != "" || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("%q emitted SQL %q or unexpected error %v", version, sql, err)
		}
	}
}

func TestBlockCorruptionsKeepsExactRangesAndContainerState(t *testing.T) {
	const version = "23.26.1.0.0"
	null := session.Value{Null: true}
	bigChange := session.Value{Text: "18446744073709551615"}
	raw := session.Result{
		Columns: corruptionColumns,
		Rows: [][]session.Value{
			{{Text: "3"}, {Text: "50"}, {Text: "3"}, bigChange, {Text: "LOGICAL"}, {Text: "4"}},
			{{Text: "3"}, {Text: "99"}, {Text: "1"}, {Text: "0"}, {Text: "CHECKSUM"}, null},
			{{Text: "3"}, {Text: "99"}, {Text: "1"}, {Text: "0"}, {Text: "CHECKSUM"}, null},
		},
	}
	decoded, err := DecodeBlockCorruptions(raw, version)
	if err != nil || decoded.Provenance.Complete || !slices.Equal(decoded.Provenance.OptionalColumns, []string{"CON_ID"}) {
		t.Fatalf("modern decode: %+v %v", decoded, err)
	}
	if len(decoded.Rows) != 3 || decoded.Rows[0].BlockID.Text != "50" || decoded.Rows[0].Blocks.Text != "3" ||
		decoded.Rows[0].CorruptionChange != bigChange || decoded.Rows[0].ContainerID == nil || decoded.Rows[0].ContainerID.Text != "4" ||
		decoded.Rows[1].CorruptionChange.Text != "0" || decoded.Rows[1].CorruptionType.Text != "CHECKSUM" ||
		decoded.Rows[1].ContainerID == nil || !decoded.Rows[1].ContainerID.Null {
		t.Fatalf("corruption or NULL scope changed: %+v", decoded.Rows)
	}
	legacy := session.Result{Columns: corruptionColumns[:5], Rows: [][]session.Value{raw.Rows[1][:5]}}
	old, err := DecodeBlockCorruptions(legacy, "9.2.0.8.0")
	if err != nil || len(old.Rows) != 1 || old.Rows[0].ContainerID != nil || len(old.Provenance.OptionalColumns) != 0 {
		t.Fatalf("legacy shape gained a container: %+v %v", old, err)
	}

	req := BlockCorruptionsRequest{OracleVersion: version, Reference: sourceRef(t)}
	empty, err := ReadBlockCorruptions(t.Context(), &fixtureReader{result: session.Result{Columns: corruptionColumns}}, req)
	if err != nil || !empty.Provenance.Complete || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("complete empty read was not distinguished: %+v %v", empty, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped, err := ReadBlockCorruptions(ctx, &fixtureReader{result: raw, after: cancel}, req)
	if !errors.Is(err, context.Canceled) || stopped.Rows != nil || stopped.Provenance.Complete {
		t.Fatalf("cancellation returned a usable range: %+v %v", stopped, err)
	}
	raw.Rows[2][3] = session.Value{Text: "1.5"}
	bad, err := ReadBlockCorruptions(t.Context(), &fixtureReader{result: raw}, req)
	if !errors.Is(err, ErrInvalidEvidence) || bad.Rows != nil || bad.Provenance.Complete {
		t.Fatalf("malformed late row yielded a prefix: %+v %v", bad, err)
	}
}

func TestBlockCorruptionsNativeRowsAndJSON(t *testing.T) {
	const version = "23.26.1.0.0"
	const change = "18446744073709551615"
	statement, err := BlockCorruptionsStatement(version)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{SQL: statement, Columns: corruptionColumns, Values: [][]driver.Value{
		{"3", "100", "2", change, "LOGICAL", int64(0)},
		{"3", "200", "9", "0", "CHECKSUM", nil},
	}}
	db, state := testsql.New(t, step)
	rows, err := BlockCorruptionsRows(t.Context(), db, version)
	if err != nil {
		t.Fatal(err)
	}
	var file, block, blocks, scn, kind string
	var container int64
	if !rows.Next() || rows.Scan(&file, &block, &blocks, &scn, &kind, &container) != nil ||
		file != "3" || block != "100" || blocks != "2" || scn != change || kind != "LOGICAL" || container != 0 {
		_ = rows.Close()
		t.Fatal("native cursor did not preserve the first range")
	}
	if err := rows.Close(); err != nil || state.Closed() != 1 {
		t.Fatalf("caller close did not reach the driver: %v, closed=%d", err, state.Closed())
	}
	db, state = testsql.New(t, step)
	limits := rowset.Limits{MaxRows: 2, MaxBytes: 8192}
	payload, err := BlockCorruptionsJSON(t.Context(), db, version, limits)
	var envelope struct {
		Provenance Provenance
		Data       struct{ Rows [][]any }
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		state.Closed() != 1 || len(envelope.Data.Rows) != 2 || envelope.Data.Rows[0][3] != change ||
		envelope.Data.Rows[0][5] != float64(0) || envelope.Data.Rows[1][5] != nil {
		t.Fatalf("JSON lost recorded evidence: %s %v", payload, err)
	}
	for _, tc := range []struct {
		step   testsql.Step
		budget rowset.Limits
		cause  error
	}{
		{step, rowset.Limits{MaxRows: 1, MaxBytes: 8192}, rowset.ErrCapacity},
		{step, rowset.Limits{MaxRows: 2, MaxBytes: len(payload) - 1}, rowset.ErrCapacity},
		{testsql.Step{SQL: statement, Columns: corruptionColumns, Values: step.Values, CloseErr: io.ErrUnexpectedEOF}, limits, io.ErrUnexpectedEOF},
	} {
		db, state := testsql.New(t, tc.step)
		payload, err := BlockCorruptionsJSON(t.Context(), db, version, tc.budget)
		if !errors.Is(err, tc.cause) || payload != nil || state.Closed() != 1 {
			t.Fatalf("failed JSON published evidence: %s, %v", payload, err)
		}
	}
}
