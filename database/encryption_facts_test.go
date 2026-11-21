package database

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

func TestEncryptionSourcesAreIndependentAndScoped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		major  int
		root   bool
		kind   EncryptionKind
		source string
	}{
		{"legacy_tablespaces", 11, false, TablespaceEncryption, "DBA_TABLESPACES"},
		{"pdb_columns", 19, false, ColumnEncryption, "DBA_ENCRYPTED_COLUMNS"},
		{"root_tablespaces", 19, true, TablespaceEncryption, "CDB_TABLESPACES"},
		{"root_columns", 26, true, ColumnEncryption, "CDB_ENCRYPTED_COLUMNS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, err := EncryptionStatementWithOptions(EncryptionOptions{OracleMajor: tc.major, Root: tc.root, Kind: tc.kind})
			if err != nil || !strings.Contains(sql, "FROM "+tc.source) || strings.Contains(sql, "V$ENCRYPTED_TABLESPACES") {
				t.Fatal("wrong scope or mounted-unsafe fallback", sql, err)
			}
			reader := encryptionFixture(coreResult("ENCRYPTED_COUNT,UNKNOWN_COUNT", coreText("0"), coreText("0")), nil)
			got, err := ReadEncryption(t.Context(), reader, EncryptionRequest{OracleMajor: tc.major, CDBRoot: tc.root, Kind: tc.kind, Reference: coreReference(t)})
			if err != nil || got.EncryptedCount.Text != "0" || got.Provenance.Source != tc.source {
				t.Fatal("measured zero or source lost", got, err)
			}
		})
	}
}
func TestEncryptionRejectsInvalidOrFailedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   session.Result
		queryErr error
	}{
		{"missing_aggregate", session.Result{Columns: []string{"ENCRYPTED_COUNT", "UNKNOWN_COUNT"}}, nil},
		{"null_count", coreResult("ENCRYPTED_COUNT,UNKNOWN_COUNT", session.Value{Null: true}, coreText("0")), nil},
		{"negative", coreResult("ENCRYPTED_COUNT,UNKNOWN_COUNT", coreText("-1"), coreText("0")), nil},
		{"late_extra_row", session.Result{Columns: []string{"ENCRYPTED_COUNT", "UNKNOWN_COUNT"}, Rows: [][]session.Value{{coreText("1"), coreText("0")}, {coreText("2"), coreText("0")}}}, nil},
		{"denied", coreResult("ENCRYPTED_COUNT,UNKNOWN_COUNT", coreText("1"), coreText("0")), errors.New("private ORA-01031")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := encryptionFixture(tc.result, tc.queryErr)
			got, err := ReadEncryption(t.Context(), reader, EncryptionRequest{OracleMajor: 19, Kind: ColumnEncryption, Reference: coreReference(t)})
			if err == nil || got.Provenance.Source != "" || got.EncryptedCount.Text != "" {
				t.Fatal("failed count published a prefix", got, err)
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("private driver detail leaked")
			}
		})
	}
	if _, err := EncryptionStatement(11, true, TablespaceEncryption); err == nil {
		t.Fatal("11g accepted CDB-root scope")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reader := encryptionFixture(coreResult("ENCRYPTED_COUNT,UNKNOWN_COUNT", coreText("0"), coreText("0")), nil)
	_, err := ReadEncryption(ctx, reader, EncryptionRequest{OracleMajor: 19, Kind: ColumnEncryption, Reference: coreReference(t)})
	if !errors.Is(err, context.Canceled) || reader.calls != 0 {
		t.Fatal("cancelled request queried", err, reader.calls)
	}
}
func TestEncryptionJSONOwnsCursorAndPublishesNoFailurePrefix(t *testing.T) {
	sql, _ := EncryptionStatement(19, true, ColumnEncryption)
	failure := errors.New("terminal failure")
	for _, tc := range []struct {
		name        string
		next, close error
		value       string
		fail        bool
	}{
		{"zero", nil, nil, "0", false}, {"late_error", failure, nil, "5", true}, {"close_error", nil, failure, "5", true}, {"invalid_domain", nil, nil, "-1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, testsql.Step{SQL: sql, Columns: []string{"ENCRYPTED_COUNT", "UNKNOWN_COUNT"}, Values: [][]driver.Value{{tc.value, "0"}}, NextErr: tc.next, CloseErr: tc.close})
			payload, err := EncryptionJSONWithOptions(t.Context(), db, EncryptionOptions{OracleMajor: 19, Root: true, Kind: ColumnEncryption}, rowset.Limits{MaxRows: 2, MaxBytes: 4096})
			if tc.fail {
				if err == nil || len(payload) != 0 {
					t.Fatal("failed JSON published", string(payload), err)
				}
			} else {
				var envelope struct {
					Data struct{ Rows [][]json.RawMessage }
				}
				if decodeErr := json.Unmarshal(payload, &envelope); err != nil || decodeErr != nil || len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 2 || string(envelope.Data.Rows[0][0]) != "0" {
					t.Fatal("numeric measured zero lost", string(payload), err, decodeErr)
				}
			}
			if state.Closed() != 1 {
				t.Fatal("JSON did not own cursor close", state.Closed())
			}
		})
	}
}

func encryptionFixture(result session.Result, err error) *coreFixtureReader {
	return &coreFixtureReader{query: func(context.Context, session.Statement) (session.Result, error) { return result, err }}
}
