package objects

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

func TestTimeZoneColumnsReleaseProjection(t *testing.T) {
	for _, tc := range []struct {
		version   string
		container bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false}, {"11.1.0.7.0", false}, {"11.2.0.4.0", false},
		{"12.1.0.1.0", true}, {"12.1.0.2.0", true}, {"12.2.0.1.0", true}, {"18.1.0.0.0", true}, {"19.32.0.0.0", true},
		{"21.21.0.0.0", true}, {"23.9.0.0.0", true}, {"23.26.1.0.0", true}, {"23.26.4.0.0", true},
	} {
		statement, err := TimeZoneColumnsStatement(TimeZoneColumnsOptions{OracleVersion: tc.version})
		if err != nil || !strings.Contains(statement, "FROM SYS.DBA_TAB_COLUMNS v") ||
			!strings.Contains(statement, "v.DATA_TYPE_OWNER IS NULL") ||
			!strings.Contains(statement, "LIKE 'TIMESTAMP(_) WITH TIME ZONE'") ||
			!strings.Contains(statement, "LIKE 'TIMESTAMP(_) WITH LOCAL TIME ZONE'") ||
			strings.Contains(statement, " AS SOURCE_CON_ID") != tc.container {
			t.Fatalf("version %s selected the wrong statement %q: %v", tc.version, statement, err)
		}
	}
	for _, version := range []string{"9.1.0.0.0", "", "26ai", "26.1.0.0.0"} {
		statement, err := TimeZoneColumnsStatement(TimeZoneColumnsOptions{OracleVersion: version})
		if statement != "" || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("unreviewed version %s returned %q: %v", version, statement, err)
		}
	}
}

func TestTimeZoneColumnsPreservesDeclarationAndNullableOrdinal(t *testing.T) {
	for _, suffix := range []string{" WITH TIME ZONE", " WITH LOCAL TIME ZONE"} {
		for precision := 0; precision <= 9; precision++ {
			declaration := "TIMESTAMP(" + strconv.Itoa(precision) + ")" + suffix
			input := session.Result{
				Columns: []string{"OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_TYPE", "COLUMN_ID", "SOURCE_CON_ID"},
				Rows:    [][]session.Value{{{Text: "AppOwner"}, {Text: "My Table"}, {Text: "Created At"}, {Text: declaration}, {Null: true}, {Text: "3"}}},
			}
			options := TimeZoneColumnsOptions{OracleVersion: "23.26.1.0.0"}
			got, err := DecodeTimeZoneColumns(input, options)
			if err != nil || len(got.Rows) != 1 || got.Provenance.Complete || got.Provenance.Scope != "connected_dictionary" || got.Provenance.Source != "DBA_TAB_COLUMNS" {
				t.Fatalf("decode %+v: %v", got, err)
			}
			row := got.Rows[0]
			if row.Owner.Text != "AppOwner" || row.TableName.Text != "My Table" || row.ColumnName.Text != "Created At" ||
				row.DataType.Text != declaration || !row.ColumnID.Null || row.SourceContainerID == nil || row.SourceContainerID.Text != "3" {
				t.Fatalf("altered identity, type, NULL or scope: %+v", row)
			}
		}
		plain := session.Result{
			Columns: []string{"OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_TYPE", "COLUMN_ID"},
			Rows:    [][]session.Value{{{Text: "APP"}, {Text: "T"}, {Text: "C"}, {Text: "TIMESTAMP" + suffix}, {Text: "2"}}},
		}
		got, err := DecodeTimeZoneColumns(plain, TimeZoneColumnsOptions{OracleVersion: "9.2.0.8.0"})
		if err != nil || len(got.Rows) != 1 || got.Rows[0].SourceContainerID != nil || got.Rows[0].DataType.Text != "TIMESTAMP"+suffix {
			t.Fatalf("legacy type/metadata changed: %+v, %v", got, err)
		}
	}
}

func TestTimeZoneColumnsRejectsLateInvalidTypesAndBounds(t *testing.T) {
	opts := TimeZoneColumnsOptions{OracleVersion: "19.32.0.0.0"}
	plan, err := timeZoneColumnsPlan(opts)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_TYPE", "COLUMN_ID", "SOURCE_CON_ID"}
	valid := []session.Value{{Text: "APP"}, {Text: "T"}, {Text: "C"}, {Text: "TIMESTAMP(6) WITH TIME ZONE"}, {Text: "1"}, {Text: "3"}}
	for _, declaration := range []string{"DATE", "TIMESTAMP(6)", "TIMESTAMP(10) WITH TIME ZONE", "TIMESTAMP(x) WITH LOCAL TIME ZONE", "TIMESTAMP(6) WITH TIME ZONE EXTRA", ""} {
		bad := append([]session.Value(nil), valid...)
		bad[3] = session.Value{Text: declaration}
		input := session.Result{Columns: names, Rows: [][]session.Value{valid, bad}}
		got, err := DecodeTimeZoneColumns(input, opts)
		if !errors.Is(err, ErrInvalidEvidence) || got.Rows != nil {
			t.Fatalf("accepted malformed late type %q: %+v, %v", declaration, got, err)
		}
	}
	req := TimeZoneColumnsRequest{Options: opts, Reference: coreReference(t), Limits: rowset.Limits{MaxRows: 1, MaxBytes: 1024}}
	reader := datasetTestReader(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
		if ctx != t.Context() || stmt.Text != plan.sql || stmt.Reference != req.Reference || len(stmt.Args) != 0 {
			t.Fatalf("unexpected admitted query: %+v", stmt)
		}
		return session.Result{Columns: names, Rows: [][]session.Value{valid, valid}}, nil
	})
	if got, err := ReadTimeZoneColumns(t.Context(), reader, req); !errors.Is(err, rowset.ErrCapacity) || got.Rows != nil {
		t.Fatalf("over-budget read returned %+v, %v", got, err)
	}
	empty := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		return session.Result{Columns: names}, nil
	})
	if got, err := ReadTimeZoneColumns(t.Context(), empty, req); err != nil || got.Rows == nil || len(got.Rows) != 0 || !got.Provenance.Complete {
		t.Fatalf("empty read changed: %+v, %v", got, err)
	}
}

func TestTimeZoneColumnsJSONPreservesTypeAndFinalizes(t *testing.T) {
	opts := TimeZoneColumnsOptions{OracleVersion: "23.26.1.0.0"}
	statement, err := TimeZoneColumnsStatement(opts)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{
		SQL: statement, Columns: []string{"OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_TYPE", "COLUMN_ID", "SOURCE_CON_ID"},
		Values: [][]driver.Value{{"APP", "T", "C", "TIMESTAMP(9) WITH LOCAL TIME ZONE", nil, int64(3)}},
	}
	db, state := testsql.New(t, step)
	payload, err := TimeZoneColumnsJSON(t.Context(), db, opts, rowset.Limits{MaxRows: 1, MaxBytes: 4096})
	var envelope struct {
		Provenance DatasetProvenance `json:"provenance"`
		Data       struct {
			Rows [][]json.RawMessage `json:"rows"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 6 ||
		string(envelope.Data.Rows[0][3]) != `"TIMESTAMP(9) WITH LOCAL TIME ZONE"` ||
		string(envelope.Data.Rows[0][4]) != "null" || string(envelope.Data.Rows[0][5]) != "3" || state.Closed() != 1 {
		t.Fatalf("distorted or incomplete JSON: %s, %v, closes=%d", payload, err, state.Closed())
	}
	cause := errors.New("private terminal failure")
	step.NextErr = cause
	db, state = testsql.New(t, step)
	payload, err = TimeZoneColumnsJSON(t.Context(), db, opts, rowset.Limits{})
	if payload != nil || !errors.Is(err, cause) || strings.Contains(err.Error(), cause.Error()) || state.Closed() != 1 {
		t.Fatalf("invalid partial JSON: %s, %v, closes=%d", payload, err, state.Closed())
	}
}
