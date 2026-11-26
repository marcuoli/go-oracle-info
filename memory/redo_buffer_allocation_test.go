package memory_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestRedoBufferAllocationSource(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23} {
		options := memory.RedoBufferAllocationOptions{OracleMajor: major, Filter: memory.InstanceFilter{Instances: []int64{3}}}
		sql, args, err := memory.RedoBufferAllocationStatement(options)
		if err != nil || !strings.Contains(sql, "v.VALUE AS BYTES FROM GV$SGA v WHERE v.NAME = 'Redo Buffers'") ||
			strings.Contains(sql, "v.CON_ID = 0") != (major >= 12) ||
			!strings.Contains(sql, "v.INST_ID IN (") || len(args) != 1 || args[0] != int64(3) {
			t.Fatalf("major %d: %s args=%v, %v", major, sql, args, err)
		}
	}
	for _, major := range []int{8, 13, 20, 22, 26} {
		sql, _, err := memory.RedoBufferAllocationStatement(memory.RedoBufferAllocationOptions{OracleMajor: major})
		if sql != "" || !errors.Is(err, memory.ErrUnsupportedVersion) {
			t.Fatalf("undocumented numeric major %d: %q, %v", major, sql, err)
		}
	}
}

func TestDecodeRedoBufferAllocationPreservesExactValues(t *testing.T) {
	options := memory.RedoBufferAllocationOptions{OracleMajor: 19, Filter: memory.InstanceFilter{Instances: []int64{2}}}
	exact := []session.Value{{Text: "2"}, {Text: "9007199254740993"}}
	null := []session.Value{{Text: "2"}, {Null: true}}
	raw := session.Result{Columns: []string{"INST_ID", "BYTES"}, Rows: [][]session.Value{exact, null, exact}}
	result, err := memory.DecodeRedoBufferAllocation(raw, options)
	if err != nil || result.Provenance.Complete || result.Provenance.Source != "GV$SGA" ||
		!slices.Equal(result.Rows, []memory.RedoBufferAllocationRow{
			{InstanceID: exact[0], Bytes: exact[1]}, {InstanceID: null[0], Bytes: null[1]}, {InstanceID: exact[0], Bytes: exact[1]},
		}) {
		t.Fatalf("exact values, NULL or duplicates lost: %#v %v", result, err)
	}
	raw.Rows = nil
	if empty, err := memory.DecodeRedoBufferAllocation(raw, options); err != nil || len(empty.Rows) != 0 {
		t.Fatalf("empty source lost: %#v, %v", empty, err)
	}
	for _, row := range [][]session.Value{
		{{Text: "1"}, {Text: "3"}}, {{Null: true}, {Text: "3"}}, {{Text: "2"}, {Text: "-1"}}, {{Text: "2"}, {Text: "1.5"}},
	} {
		raw.Rows = [][]session.Value{exact, row}
		if bad, err := memory.DecodeRedoBufferAllocation(raw, options); !errors.Is(err, memory.ErrInvalidObservedEvidence) || len(bad.Rows) != 0 {
			t.Fatalf("invalid sample leaked a prefix: %#v %v", bad, err)
		}
	}
}
