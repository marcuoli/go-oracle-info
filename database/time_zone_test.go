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

func TestTimeZoneStatementVersionAdmission(t *testing.T) {
	const statement = "SELECT DBTIMEZONE AS DB_TIME_ZONE FROM SYS.DUAL"
	for _, version := range []string{
		"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0",
		"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.1.0.0.0", "19.32.0.0.0",
		"21.21.0.0.0", "23.9.0.0.0", "23.26.1.0.0", "23.26.4.0.0",
	} {
		got, err := TimeZoneStatement(TimeZoneOptions{OracleVersion: version})
		if err != nil || got != statement {
			t.Fatalf("version %s selected %q: %v", version, got, err)
		}
	}
	for _, version := range []string{"", "9.1.0", "10.3.0", "26ai", "26.1.0.0.0"} {
		request := TimeZoneRequest{Options: TimeZoneOptions{OracleVersion: version}, Reference: coreReference(t)}
		reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
			t.Fatal("unsupported version reached query")
			return session.Result{}, nil
		})
		facts, err := ReadTimeZone(t.Context(), reader, request)
		if !errors.Is(err, ErrUnsupportedVersion) || facts.Rows != nil {
			t.Fatalf("unsupported version %q returned %+v, %v", version, facts, err)
		}
	}
}

func TestTimeZoneReadExactTextNullAndEmpty(t *testing.T) {
	request := TimeZoneRequest{
		Options: TimeZoneOptions{OracleVersion: "23.26.1.0.0"}, Reference: coreReference(t),
	}
	for _, tc := range []struct {
		name string
		rows [][]session.Value
	}{
		{"offset", [][]session.Value{{{Text: "+05:45"}}}},
		{"region", [][]session.Value{{{Text: "America/Sao_Paulo"}}}},
		{"null", [][]session.Value{{{Null: true}}}},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := session.Result{Columns: []string{"DB_TIME_ZONE"}, Rows: tc.rows}
			reader := datasetTestReader(func(ctx context.Context, stmt session.Statement) (session.Result, error) {
				if ctx != t.Context() || stmt.Text != "SELECT DBTIMEZONE AS DB_TIME_ZONE FROM SYS.DUAL" || len(stmt.Args) != 0 || stmt.Reference != request.Reference {
					t.Fatalf("incorrect DBTIMEZONE request: %+v", stmt)
				}
				return source, nil
			})
			facts, err := ReadTimeZone(t.Context(), reader, request)
			want := DatasetProvenance{Source: "DBTIMEZONE", OracleMajor: 23, Scope: "connected_database_time_zone", Complete: true}
			if err != nil || facts.Provenance != want || facts.Rows == nil || len(facts.Rows) != len(tc.rows) {
				t.Fatalf("read %+v: %v", facts, err)
			}
			if len(tc.rows) == 1 && facts.Rows[0].Value != tc.rows[0][0] {
				t.Fatalf("altered Oracle text or NULL: %+v", facts.Rows[0])
			}
		})
	}
}

func TestTimeZoneReadRejectsPartialAndMalformedEvidence(t *testing.T) {
	request := TimeZoneRequest{Options: TimeZoneOptions{OracleVersion: "19.32.0.0.0"}, Reference: coreReference(t)}
	good := coreResult("DB_TIME_ZONE", session.Value{Text: "-03:00"})
	denied := errors.New("private Oracle cause")
	for _, tc := range []struct {
		name     string
		result   session.Result
		queryErr error
		limits   rowset.Limits
		want     error
	}{
		{"wrong alias", coreResult("SESSIONTIMEZONE", session.Value{Text: "-03:00"}), nil, rowset.Limits{}, ErrInvalidEvidence},
		{"incoherent NULL", coreResult("DB_TIME_ZONE", session.Value{Null: true, Text: "-03:00"}), nil, rowset.Limits{}, ErrInvalidEvidence},
		{"late row", session.Result{Columns: good.Columns, Rows: append(good.Rows, good.Rows[0])}, nil, rowset.Limits{}, ErrInvalidEvidence},
		{"access failure", good, denied, rowset.Limits{}, denied},
		{"capacity", good, nil, rowset.Limits{MaxRows: 1, MaxBytes: 5}, rowset.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bounded := request
			bounded.Limits = tc.limits
			reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
				return tc.result, tc.queryErr
			})
			facts, err := ReadTimeZone(t.Context(), reader, bounded)
			if !errors.Is(err, tc.want) || facts.Rows != nil || facts.Provenance.Complete || strings.Contains(err.Error(), denied.Error()) {
				t.Fatalf("failed read returned %+v: %v", facts, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := ReadTimeZone(ctx, datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return good, nil
	}), request)
	if !errors.Is(err, context.Canceled) || result.Rows != nil {
		t.Fatalf("canceled read returned %+v: %v", result, err)
	}
}

func TestTimeZoneJSONValidatesAndFinalizesCursor(t *testing.T) {
	options := TimeZoneOptions{OracleVersion: "23.26.1.0.0"}
	step := testsql.Step{
		SQL:     "SELECT DBTIMEZONE AS DB_TIME_ZONE FROM SYS.DUAL",
		Columns: []string{"DB_TIME_ZONE"},
		Values:  [][]driver.Value{{nil}},
	}
	db, state := testsql.New(t, step)
	payload, err := TimeZoneJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance DatasetProvenance `json:"provenance"`
		Data       struct {
			Rows [][]json.RawMessage `json:"rows"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 1 || string(envelope.Data.Rows[0][0]) != "null" || state.Closed() != 1 {
		t.Fatalf("JSON lost source NULL or failed to complete: %s, %v, closes=%d", payload, err, state.Closed())
	}
	terminal := errors.New("private terminal cause")
	step.NextErr = terminal
	db, state = testsql.New(t, step)
	payload, err = TimeZoneJSON(t.Context(), db, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, terminal) || state.Closed() != 1 {
		t.Fatalf("terminal failure returned %s: %v, closes=%d", payload, err, state.Closed())
	}
}
