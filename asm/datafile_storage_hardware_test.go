package asm

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestDatafileStorageHardwareReleaseSelection(t *testing.T) {
	for _, tc := range []struct {
		version string
		major   int
	}{
		{"9.2.0.8.0", 0}, {"10.1.0.5.0", 0}, {"10.2.0.1.0", 10},
		{"11.1.0.6.0", 11}, {"11.2.0.4.0", 11}, {"12.1.0.2.0", 12},
		{"12.2.0.1.0", 12}, {"18.1.0.0.0", 18}, {"19.32.0.0.0", 19},
		{"21.1.0.0.0", 21}, {"23.4.0.0.0", 23}, {"23.9.0.0.0", 23},
		{"23.26.1.0.0", 23}, {"26.1.0.0.0", 0}, {"26ai", 0},
	} {
		t.Run(tc.version, func(t *testing.T) {
			stmt, err := DatafileStorageHardwareStatement(tc.version)
			if tc.major == 0 {
				reader := &fixtureReader{}
				_, readErr := ReadDatafileStorageHardware(context.Background(), reader, DatafileStorageHardwareRequest{OracleVersion: tc.version})
				if stmt != "" || !errors.Is(err, ErrUnsupportedVersion) || !errors.Is(readErr, ErrUnsupportedVersion) || len(reader.calls) != 0 {
					t.Fatalf("unsupported version queried: %q, %v, %v, %d calls", stmt, err, readErr, len(reader.calls))
				}
				return
			}
			if err != nil || !strings.Contains(stmt, "FROM V$ASM_DISK_STAT d\nJOIN V$ASM_DISKGROUP_STAT g ON g.GROUP_NUMBER = d.GROUP_NUMBER") ||
				!strings.Contains(stmt, "d.PATH LIKE 'o/%'") || !strings.Contains(stmt, "FROM V$DATAFILE f") ||
				!strings.Contains(stmt, "LENGTH(g.NAME) + 2)) = '+' || UPPER(g.NAME) || '/'") {
				t.Fatalf("missing disk, datafile or exact group boundary: %q, %v", stmt, err)
			}
			source := session.Result{Columns: []string{"HARDWARE_TYPE", "EXADATA_DISK_COUNT"}, Rows: [][]session.Value{{{Text: "UNKNOWN"}, {Text: "0"}}}}
			fact, err := DecodeDatafileStorageHardware(source, tc.version)
			if err != nil || fact.HardwareType != StorageHardwareUnknown || fact.Provenance.OracleMajor != tc.major || fact.Provenance.Complete {
				t.Fatalf("zero or provenance lost: %+v, %v", fact, err)
			}
		})
	}
}

func TestDatafileStorageHardwareRejectsInconsistentEvidence(t *testing.T) {
	for _, tc := range []struct {
		count session.Value
		label session.Value
		want  StorageHardwareType
	}{
		{session.Value{Text: "20"}, session.Value{Text: "EXADATA_STORAGE"}, StorageHardwareExadata},
		{session.Value{Text: "900719925474099312345"}, session.Value{Text: "EXADATA_STORAGE"}, StorageHardwareExadata},
		{session.Value{Text: "0"}, session.Value{Text: "UNKNOWN"}, StorageHardwareUnknown},
		{session.Value{Text: "0"}, session.Value{Text: "EXADATA_STORAGE"}, ""},
		{session.Value{Text: "20"}, session.Value{Text: "UNKNOWN"}, ""},
		{session.Value{Text: "-1"}, session.Value{Text: "UNKNOWN"}, ""},
		{session.Value{Null: true}, session.Value{Text: "UNKNOWN"}, ""},
		{session.Value{Text: "0"}, session.Value{Null: true}, ""},
	} {
		in := session.Result{Columns: []string{"EXADATA_DISK_COUNT", "HARDWARE_TYPE"}, Rows: [][]session.Value{{tc.count, tc.label}}}
		out, err := DecodeDatafileStorageHardware(in, "19.32.0.0.0")
		if tc.want == "" {
			if out.HardwareType != "" || out.Provenance.Complete || !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("malformed evidence %+v: %+v, %v", tc, out, err)
			}
			continue
		}
		if err != nil || out.HardwareType != tc.want || out.ExadataDiskCount != tc.count {
			t.Fatalf("evidence %+v: %+v, %v", tc, out, err)
		}
	}
}

func TestDatafileStorageHardwareReaderAndNativePaths(t *testing.T) {
	ctx := context.Background()
	version := "19.32.0.0.0"
	query, err := DatafileStorageHardwareStatement(version)
	if err != nil {
		t.Fatal(err)
	}
	cols := []string{"HARDWARE_TYPE", "EXADATA_DISK_COUNT"}
	row := []session.Value{{Text: "EXADATA_STORAGE"}, {Text: "20"}}
	reader := &fixtureReader{result: session.Result{Columns: cols, Rows: [][]session.Value{row}}}
	request := DatafileStorageHardwareRequest{OracleVersion: version, Reference: sourceRef(t)}
	out, err := ReadDatafileStorageHardware(ctx, reader, request)
	if err != nil || !out.Provenance.Complete || out.HardwareType != StorageHardwareExadata ||
		out.Provenance.Source != "V$ASM_DISK_STAT" || len(reader.calls) != 1 || reader.calls[0].Text != query {
		t.Fatalf("read: %+v, %v, calls %d", out, err, len(reader.calls))
	}
	reader.result.Rows = nil
	if out, err := ReadDatafileStorageHardware(ctx, reader, request); out.HardwareType != "" || !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("missing aggregate accepted: %+v, %v", out, err)
	}

	step := testsql.Step{SQL: query, Columns: cols, Values: [][]driver.Value{{"EXADATA_STORAGE", int64(20)}}}
	db, tracker := testsql.New(t, step, step)
	rows, err := DatafileStorageHardwareRows(ctx, db, version)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var hardware string
	var count int64
	if !rows.Next() {
		t.Fatalf("no native row: %v", rows.Err())
	}
	if err := rows.Scan(&hardware, &count); err != nil || hardware != "EXADATA_STORAGE" || count != 20 {
		t.Fatalf("native row %s/%d, %v", hardware, count, err)
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected native completion: %v", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := DatafileStorageHardwareJSON(ctx, db, version, rowset.Limits{})
	var envelope struct {
		Data struct {
			Rows [][]json.RawMessage
		}
		Provenance Provenance
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 2 ||
		string(envelope.Data.Rows[0][0]) != `"EXADATA_STORAGE"` || string(envelope.Data.Rows[0][1]) != "20" || tracker.Closed() != 2 {
		t.Fatalf("bounded JSON: %s, %v, closed %d", payload, err, tracker.Closed())
	}

	terminalErr := errors.New("terminal driver error")
	step.NextErr = terminalErr
	failedDB, failedTracker := testsql.New(t, step)
	if payload, err := DatafileStorageHardwareJSON(ctx, failedDB, version, rowset.Limits{}); payload != nil || !errors.Is(err, terminalErr) || failedTracker.Closed() != 1 {
		t.Fatalf("terminal failure leaked result: %s, %v, closed %d", payload, err, failedTracker.Closed())
	}
}
