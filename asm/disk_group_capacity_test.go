package asm

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

func TestDiskGroupCapacityReleaseAndRoleSelection(t *testing.T) {
	legacy := []string{"GROUP_NUMBER", "NAME", "STATE", "TYPE", "TOTAL_MB", "FREE_MB", "REQUIRED_MIRROR_FREE_MB", "USABLE_FILE_MB"}
	for _, tc := range []struct {
		version string
		columns int
	}{
		{"9.2.0.8.0", 0}, {"10.1.0.5.0", 0}, {"10.2.0.1.0", 8}, {"11.1.0.6.0", 8}, {"11.2.0.4.0", 8},
		{"12.1.0.1.0", 9}, {"12.2.0.1.0", 9}, {"18.1.0.0.0", 9}, {"19.32.0.0.0", 9}, {"21.1.0.0.0", 9},
		{"23.9.0.0.0", 9}, {"23.26.1.0.0", 9}, {"23.26.4.0.0", 9}, {"26.1.0.0.0", 0}, {"26ai", 0},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := DiskGroupCapacityOptions{OracleVersion: tc.version, SourceRole: DatabaseInstance}
			statement, err := DiskGroupCapacityStatement(options)
			if tc.columns == 0 {
				reader := &fixtureReader{}
				_, readErr := ReadDiskGroupCapacity(context.Background(), reader, DiskGroupCapacityRequest{Options: options})
				if !errors.Is(err, ErrUnsupportedVersion) || statement != "" || !errors.Is(readErr, ErrUnsupportedVersion) || len(reader.calls) != 0 {
					t.Fatalf("unsupported version: SQL %q, selector %v, read %v, calls %d", statement, err, readErr, len(reader.calls))
				}
				return
			}
			want := legacy
			if tc.columns == 9 {
				want = append(slices.Clone(legacy), "CON_ID")
			}
			if err != nil || !strings.HasSuffix(statement, " FROM V$ASM_DISKGROUP_STAT v ORDER BY v.GROUP_NUMBER, v.NAME") {
				t.Fatalf("wrong statement %q: %v", statement, err)
			}
			for _, column := range want {
				if !strings.Contains(statement, " AS "+column) {
					t.Fatalf("missing alias %s in %q", column, statement)
				}
			}
			empty := session.Result{Columns: want}
			result, err := DecodeDiskGroupCapacity(empty, options)
			if err != nil || result.Rows == nil || len(result.Rows) != 0 || result.Provenance.Complete || result.Provenance.Scope != "database_visible_asm_disk_groups" {
				t.Fatalf("lost empty or database scope: %+v, %v", result, err)
			}
		})
	}

	if sql, err := DiskGroupCapacityStatement(DiskGroupCapacityOptions{OracleVersion: "23.26.1"}); sql != "" || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("implicit source role admitted: %q %v", sql, err)
	}
}

func TestDiskGroupCapacityExactAndAtomicDecode(t *testing.T) {
	options := DiskGroupCapacityOptions{OracleVersion: "23.26.1.0.0", SourceRole: ASMInstance}
	columns := []string{"GROUP_NUMBER", "NAME", "STATE", "TYPE", "TOTAL_MB", "FREE_MB", "REQUIRED_MIRROR_FREE_MB", "USABLE_FILE_MB", "CON_ID"}
	values := []session.Value{
		{Text: "0"}, {Text: "DATA"}, {Text: "FUTURE_STATE"}, {Text: "FLEX"},
		{Text: "900719925474099312345"}, {Null: true}, {Text: "0"}, {Text: "-900719925474099312345"}, {Text: "0"},
	}
	second := slices.Clone(values)
	second[3], second[7], second[8] = session.Value{Text: "EXTEND"}, session.Value{Text: "0"}, session.Value{Null: true}
	source := session.Result{Columns: columns, Rows: [][]session.Value{values, second}}
	result, err := DecodeDiskGroupCapacity(source, options)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiskGroupCapacity{
		{values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], &values[8]},
		{second[0], second[1], second[2], second[3], second[4], second[5], second[6], second[7], &second[8]},
	}
	if !slices.EqualFunc(result.Rows, want, func(got, expected DiskGroupCapacity) bool {
		if got.ContainerID == nil || *got.ContainerID != *expected.ContainerID {
			return false
		}
		got.ContainerID, expected.ContainerID = nil, nil
		return got == expected
	}) || result.Provenance.Scope != "asm_instance_disk_groups" || result.Provenance.SourceRole != "asm_instance" {
		t.Fatalf("lost raw, duplicate, exact numeric or NULL evidence: %+v", result)
	}
	legacy, err := DecodeDiskGroupCapacity(session.Result{Columns: columns[:8], Rows: [][]session.Value{values[:8]}},
		DiskGroupCapacityOptions{OracleVersion: "10.2.0.5.0", SourceRole: ASMInstance})
	if err != nil || len(legacy.Rows) != 1 || legacy.Rows[0].ContainerID != nil {
		t.Fatalf("legacy projection: %+v, %v", legacy, err)
	}
	for _, tc := range []struct {
		index int
		text  string
	}{{5, "-1"}, {7, "1.5"}, {8, "3"}} {
		invalid := slices.Clone(second)
		invalid[tc.index] = session.Value{Text: tc.text}
		bad, err := DecodeDiskGroupCapacity(session.Result{Columns: columns, Rows: [][]session.Value{values, invalid}}, options)
		if bad.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
			t.Fatalf("returned prefix for malformed column %s: %+v, %v", columns[tc.index], bad, err)
		}
	}
}

func TestDiskGroupCapacityReadAndJSONCompletion(t *testing.T) {
	options := DiskGroupCapacityOptions{OracleVersion: "23.26.1.0.0", SourceRole: DatabaseInstance}
	plan, err := diskGroupCapacityPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	source := sourceResult(plan)
	source.Rows[0][5], source.Rows[0][7], source.Rows[0][8] = session.Value{Null: true}, session.Value{Text: "-42"}, session.Value{Text: "0"}
	bounds := rowset.Limits{MaxRows: 1, MaxBytes: 4096}
	request := DiskGroupCapacityRequest{Options: options, Reference: sourceRef(t), Limits: bounds}
	reader := &fixtureReader{result: source}
	ctx := context.Background()
	result, err := ReadDiskGroupCapacity(ctx, reader, request)
	if err != nil || !result.Provenance.Complete || len(reader.calls) != 1 || reader.calls[0].Text != plan.sql {
		t.Fatalf("read %+v, calls %+v, error %v", result, reader.calls, err)
	}
	reader.result.Rows = append(reader.result.Rows, source.Rows[0])
	result, err = ReadDiskGroupCapacity(ctx, reader, request)
	if result.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("buffered capacity: %+v, %v", result, err)
	}

	step := testsql.Step{SQL: plan.sql, Columns: source.Columns, Values: [][]driver.Value{{
		"1", "DATA", "MOUNTED", "NORMAL", "900719925474099312345", nil, "20", "-42", int64(0),
	}}}
	db, state := testsql.New(t, step)
	payload, err := DiskGroupCapacityJSON(ctx, db, options, bounds)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance Provenance `json:"provenance"`
		Data       struct {
			Rows [][]json.RawMessage `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.Complete || envelope.Provenance.Source != "V$ASM_DISKGROUP_STAT" ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 9 ||
		string(envelope.Data.Rows[0][4]) != `"900719925474099312345"` ||
		string(envelope.Data.Rows[0][5]) != "null" || string(envelope.Data.Rows[0][7]) != `"-42"` ||
		string(envelope.Data.Rows[0][8]) != "0" || state.Closed() != 1 {
		t.Fatalf("JSON did not retain values/provenance/closure: %s, closes %d", payload, state.Closed())
	}
	sentinel := errors.New("private terminal driver text")
	step.NextErr = sentinel
	db, state = testsql.New(t, step)
	payload, err = DiskGroupCapacityJSON(ctx, db, options, bounds)
	if payload != nil || !errors.Is(err, sentinel) || strings.Contains(err.Error(), sentinel.Error()) || state.Closed() != 1 {
		t.Fatalf("terminal cursor: %s, %v, closes %d", payload, err, state.Closed())
	}
}
