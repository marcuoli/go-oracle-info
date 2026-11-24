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

func TestFileHealthReleaseProjections(t *testing.T) {
	for _, tc := range []struct {
		release   string
		container bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false}, {"12.1.0.1.0", true},
		{"12.1.0.2.0", true}, {"12.2.0.1.0", true}, {"18.1.0.0.0", true},
		{"19.32.0.0.0", true}, {"21.21.0.0.0", true}, {"23.9.0.0.0", true},
		{"23.26.1.0.0", true}, {"23.26.4.0.0", true},
	} {
		t.Run(tc.release, func(t *testing.T) {
			opts := FileHealthOptions{tc.release}
			for _, x := range []struct {
				statement func(FileHealthOptions) (string, error)
				want      []string
			}{
				{FileStatusesStatement, []string{"FROM V$DATAFILE v UNION ALL SELECT", "FROM V$TEMPFILE v", "v.STATUS AS STATUS"}},
				{FileRecoveryStatement, []string{"FROM V$RECOVER_FILE v", "v.ERROR AS ERROR", "TO_CHAR(v.CHANGE#", " AS RECOVERY_TIME"}},
				{FileBackupModesStatement, []string{"FROM GV$BACKUP v", "v.INST_ID AS INST_ID", "v.STATUS AS STATUS", "AS START_SCN"}},
				{FileHeadersStatement, []string{"FROM V$DATAFILE_HEADER v", "v.FUZZY AS FUZZY", "v.RECOVER AS RECOVER", "v.ERROR AS ERROR", "CHECKPOINT_CHANGE#", " AS CHECKPOINT_TIME"}},
			} {
				sql, err := x.statement(opts)
				if err != nil || strings.Contains(sql, "v.CON_ID AS CON_ID") != tc.container ||
					strings.Contains(sql, fileSourceContainerSQL) != tc.container {
					t.Fatalf("container projection = %q, %v", sql, err)
				}
				for _, needle := range x.want {
					if !strings.Contains(sql, needle) {
						t.Fatalf("missing %q in %q", needle, sql)
					}
				}
			}
		})
	}
	db, state := testsql.New(t)
	got, err := FileStatusesJSON(t.Context(), db, FileHealthOptions{"26ai"}, rowset.Limits{})
	if got != nil || !errors.Is(err, ErrUnsupportedVersion) || len(state.Calls()) != 0 {
		t.Fatal(got, err, state.Calls())
	}
}

func TestFileHealthTypedEvidence(t *testing.T) {
	null := session.Value{Null: true}
	v := func(s string) session.Value { return session.Value{Text: s} }
	newOpts := FileHealthOptions{"23.26.1.0.0"}
	oldOpts := FileHealthOptions{"9.2.0.8.0"}
	status := session.Result{
		Columns: []string{"FILE_ID", "INST_ID", "FILE_TYPE", "FILE_NAME", "STATUS", "CON_ID", "SOURCE_CON_ID"},
		Rows: [][]session.Value{
			{v("7"), v("1"), v("DATAFILE"), v("+DATA/f.dbf"), v("RECOVER"), v("4"), v("1")},
			{v("7"), v("1"), v("TEMPFILE"), null, v("ONLINE"), v("4"), v("1")},
		},
	}
	files, err := DecodeFileStatuses(status, newOpts)
	if err != nil || len(files.Rows) != 2 || files.Provenance.Complete || files.Rows[0].FileID.Text != "7" ||
		files.Rows[0].Status.Text != "RECOVER" || files.Rows[1].Type.Text != "TEMPFILE" || !files.Rows[1].Name.Null ||
		files.Rows[0].ContainerID.Text != "4" || files.Rows[0].SourceContainerID.Text != "1" {
		t.Fatal(files, err)
	}
	recovery := session.Result{
		Columns: []string{"FILE_ID", "INST_ID", "ERROR", "RECOVERY_SCN", "RECOVERY_TIME"},
		Rows: [][]session.Value{
			{v("7"), v("1"), v("OFFLINE NORMAL"), v("0"), null},
			{v("8"), v("1"), null, v("18446744073709551615"), v("2026-11-24T13:01:02")},
		},
	}
	recovering, err := DecodeFileRecovery(recovery, oldOpts)
	if err != nil || len(recovering.Rows) != 2 || recovering.Rows[0].Error.Text != "OFFLINE NORMAL" ||
		recovering.Rows[0].RecoverySCN.Text != "0" || !recovering.Rows[0].RecoveryTime.Null ||
		!recovering.Rows[1].Error.Null || recovering.Rows[1].RecoverySCN.Text != "18446744073709551615" ||
		recovering.Rows[1].ContainerID != nil || recovering.Rows[1].SourceContainerID != nil {
		t.Fatal(recovering, err)
	}
	backup := session.Result{
		Columns: []string{"FILE_ID", "INST_ID", "STATUS", "START_SCN", "START_TIME", "CON_ID", "SOURCE_CON_ID"},
		Rows: [][]session.Value{
			{v("7"), v("1"), v("ACTIVE"), v("9007199254740999"), v("2026-11-24T14:00:00"), v("4"), v("1")},
			{v("7"), v("2"), v("NOT ACTIVE"), null, null, v("4"), v("1")},
		},
	}
	modes, err := DecodeFileBackupModes(backup, newOpts)
	if err != nil || len(modes.Rows) != 2 || modes.Rows[0].Status.Text != "ACTIVE" ||
		modes.Rows[0].StartSCN.Text != "9007199254740999" || !modes.Rows[1].StartSCN.Null ||
		modes.Rows[1].ReportingInstanceID.Text != "2" || modes.Rows[1].Status.Text != "NOT ACTIVE" {
		t.Fatal(modes, err)
	}
	header := session.Result{
		Columns: []string{"FILE_ID", "INST_ID", "ERROR", "RECOVER", "FUZZY", "CHECKPOINT_SCN", "CHECKPOINT_TIME", "CON_ID", "SOURCE_CON_ID"},
		Rows: [][]session.Value{
			{v("7"), v("1"), null, v("NO"), v("YES"), v("9007199254740999"), v("2026-11-24T14:12:59"), v("2"), v("1")},
			{v("8"), v("1"), v("CANNOT READ"), null, null, null, null, null, v("1")},
		},
	}
	headers, err := DecodeFileHeaders(header, newOpts)
	if err != nil || len(headers.Rows) != 2 || !headers.Rows[0].Error.Null || headers.Rows[0].Recover.Text != "NO" ||
		headers.Rows[0].Fuzzy.Text != "YES" || headers.Rows[0].CheckpointSCN.Text != "9007199254740999" ||
		headers.Rows[1].Error.Text != "CANNOT READ" || !headers.Rows[1].CheckpointTime.Null || !headers.Rows[1].ContainerID.Null {
		t.Fatal(headers, err)
	}
}

func TestFileHealthCompletenessAndNativeEnvelope(t *testing.T) {
	opts := FileHealthOptions{"23.26.1.0.0"}
	plan, err := fileRecoveryPlan(opts)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Result{
		Columns: []string{"FILE_ID", "INST_ID", "ERROR", "RECOVERY_SCN", "RECOVERY_TIME", "CON_ID", "SOURCE_CON_ID"},
		Rows: [][]session.Value{
			{{Text: "7"}, {Text: "1"}, {Text: "OFFLINE NORMAL"}, {Text: "0"}, {Null: true}, {Text: "3"}, {Text: "1"}},
			{{Text: "8"}, {Text: "1"}, {Null: true}, {Text: "9007199254740999"}, {Text: "2026-11-24T01:02:03"}, {Text: "4"}, {Text: "1"}},
		},
	}
	req := FileHealthRequest{Options: opts, Reference: sourceRef(t)}
	reader := &fixtureReader{result: result}
	facts, err := ReadFileRecovery(t.Context(), reader, req)
	if err != nil || !facts.Provenance.Complete || len(facts.Rows) != 2 || len(reader.calls) != 1 ||
		reader.calls[0].Text != plan.sql || reader.calls[0].Reference != req.Reference {
		t.Fatal(facts, err, reader.calls)
	}
	req.Limits.MaxRows = 1
	if got, err := ReadFileRecovery(t.Context(), reader, req); got.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatal(got, err)
	}
	req.Limits = rowset.Limits{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if got, err := ReadFileRecovery(ctx, &fixtureReader{result: result, after: cancel}, req); got.Rows != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(got, err)
	}
	empty := session.Result{Columns: result.Columns}
	if got, err := ReadFileRecovery(t.Context(), &fixtureReader{result: empty}, req); got.Rows == nil || len(got.Rows) != 0 || err != nil || !got.Provenance.Complete {
		t.Fatal(got, err)
	}

	step := testsql.Step{SQL: plan.sql, Columns: result.Columns, Values: [][]driver.Value{
		{int64(7), int64(1), "OFFLINE NORMAL", "0", nil, int64(3), int64(1)},
		{int64(8), int64(1), nil, "9007199254740999", "2026-11-24T01:02:03", int64(4), int64(1)},
	}}
	failing := step
	failing.CloseErr = io.ErrUnexpectedEOF
	db, state := testsql.New(t, step, failing)
	payload, err := FileRecoveryJSON(t.Context(), db, opts, rowset.Limits{})
	var envelope struct {
		Provenance Provenance
		Data       struct{ Rows [][]any }
	}
	decodeErr := json.Unmarshal(payload, &envelope)
	if err != nil || decodeErr != nil || !envelope.Provenance.Complete || len(envelope.Data.Rows) != 2 ||
		envelope.Data.Rows[0][4] != nil || envelope.Data.Rows[1][3] != "9007199254740999" {
		t.Fatal(string(payload), err, decodeErr)
	}
	if got, err := FileRecoveryJSON(t.Context(), db, opts, rowset.Limits{}); got != nil || !errors.Is(err, io.ErrUnexpectedEOF) || state.Closed() != 2 {
		t.Fatal(got, err, state.Closed())
	}
	step.Values[1][4] = "2026-02-31T01:02:03"
	db, state = testsql.New(t, step)
	if got, err := FileRecoveryJSON(t.Context(), db, opts, rowset.Limits{}); got != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(got, err, state.Closed())
	}
}
