package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/rowset"
)

const legacySQL = "SELECT VERSION AS VERSION FROM V$INSTANCE"
const fullSQL = "SELECT VERSION_FULL AS VERSION FROM V$INSTANCE"

func versionSteps(major string) []testsql.Step {
	steps := []testsql.Step{{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{major + ".0.0.0.0"}}}}
	if major != "11" {
		steps = append(steps, testsql.Step{SQL: fullSQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{major + ".26.1.0.0"}}})
	}
	return steps
}
func parameterStep(modern, stored bool) testsql.Step {
	names := []string{"INST_ID"}
	if modern {
		names = append(names, "CON_ID")
	}
	if stored {
		names = append(names, "SID", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISSPECIFIED", "UPDATE_COMMENT")
	} else {
		names = append(names, "NUM", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISDEFAULT", "ISSES_MODIFIABLE", "ISSYS_MODIFIABLE")
		if modern {
			names = append(names, "ISPDB_MODIFIABLE")
		}
		names = append(names, "ISINSTANCE_MODIFIABLE", "ISMODIFIED", "ISADJUSTED", "ISDEPRECATED", "ISBASIC", "DESCRIPTION", "UPDATE_COMMENT")
	}
	values := make([]driver.Value, len(names))
	for i, name := range names {
		switch name {
		case "INST_ID":
			values[i] = int64(1)
		case "CON_ID":
			values[i] = int64(3)
		case "NUM":
			values[i] = int64(7)
		case "TYPE":
			if stored {
				values[i] = "6"
			} else {
				values[i] = int64(6)
			}
		case "ORDINAL":
			values[i] = int64(0)
		case "NAME":
			values[i] = "control_files"
		case "SID":
			values[i] = "DormantCase"
		case "VALUE":
			values[i] = nil
		case "DISPLAY_VALUE":
			values[i] = "  one,two  "
		case "UPDATE_COMMENT":
			values[i] = nil
		default:
			values[i] = "FALSE"
		}
	}
	return testsql.Step{Columns: names, Values: [][]driver.Value{values}}
}

type nativeSpy struct {
	db   *sql.DB
	last *sql.Rows
}

func (s *nativeSpy) QueryContext(ctx context.Context, sql string, args ...any) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	s.last = rows
	return rows, err
}

func TestClientReturnsUnmodifiedNativeCursor(t *testing.T) {
	t.Parallel()
	steps := append(versionSteps("23"), parameterStep(true, false))
	db, state := testsql.New(t, steps...)
	spy := &nativeSpy{db: db}
	facts := info.New(spy)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rows, err := facts.InitParameters(ctx, info.ParameterFilter{Names: []string{"control_files"}, Instances: []int64{1}})
	if err != nil || rows != spy.last || ctx.Err() != nil {
		t.Fatal("native cursor identity/lifetime lost", err)
	}
	if !rows.Next() {
		t.Fatal("returned cursor already finalized")
	}
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]any, len(columns))
	dest := make([]any, len(values))
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatal(err)
	}
	if values[0] != int64(1) || values[4] != int64(6) || values[6] != nil || values[7] != "  one,two  " {
		t.Fatal("native values transformed", values)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(state.Calls()) != 3 || state.Closed() != 3 {
		t.Fatal("version/cursor sequence not finalized")
	}
}
func TestClientJSONKeepsNativeTypesNullAndProvenance(t *testing.T) {
	t.Parallel()
	for _, stored := range []bool{false, true} {
		t.Run(map[bool]string{false: "effective", true: "stored"}[stored], func(t *testing.T) {
			steps := append(versionSteps("23"), parameterStep(true, stored))
			db, state := testsql.New(t, steps...)
			facts := info.New(db)
			var data json.RawMessage
			var err error
			if stored {
				data, err = facts.SPFileParametersJSON(t.Context(), info.SPFileFilter{Names: []string{"control_files"}, Instances: []int64{1}, SIDs: []string{"DormantCase"}})
			} else {
				data, err = facts.InitParametersJSON(t.Context(), info.ParameterFilter{Names: []string{"control_files"}, Instances: []int64{1}})
			}
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Provenance struct {
					Source, Scope, SIDPolicy string
					OracleMajor              int `json:"oracle_major"`
					SIDs                     []string
				}
				Data struct {
					Columns []rowset.Column
					Rows    [][]any
				}
			}
			decoder := json.NewDecoder(strings.NewReader(string(data)))
			decoder.UseNumber()
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Provenance.OracleMajor != 23 || envelope.Provenance.Scope != "current_container_and_whole_database_rows" || len(envelope.Data.Rows) != 1 || state.Closed() != 3 {
				t.Fatal("source/scope/lifecycle lost")
			}
			for i, col := range envelope.Data.Columns {
				value := envelope.Data.Rows[0][i]
				switch col.Name {
				case "INST_ID":
					if value != json.Number("1") {
						t.Fatal("native ID stringified")
					}
				case "TYPE":
					if stored {
						if value != "6" || col.Kind != rowset.Text {
							t.Fatal("SPFILE type converted to number")
						}
					} else if value != json.Number("6") || col.Kind != rowset.Integer {
						t.Fatal("system type stringified")
					}
				case "VALUE":
					if value != nil {
						t.Fatal("NULL replaced")
					}
				case "DISPLAY_VALUE":
					if value != "  one,two  " {
						t.Fatal("whitespace/comma normalized")
					}
				case "ISSPECIFIED":
					if value != "FALSE" {
						t.Fatal("no-SPFILE metadata inferred away")
					}
				}
			}
			calls := state.Calls()
			if len(calls) != 3 {
				t.Fatal("unexpected query count")
			}
			if stored && (len(calls[2].Args) != 4 || calls[2].Args[2].Value != "DormantCase" || calls[2].Args[3].Value != "*") {
				t.Fatal("wildcard/dormant applicability lost")
			}
		})
	}
}
func TestClientDoesNotCacheCapabilitiesAcrossCalls(t *testing.T) {
	t.Parallel()
	steps := append(versionSteps("11"), parameterStep(false, false))
	steps = append(steps, versionSteps("23")...)
	steps = append(steps, parameterStep(true, false))
	db, state := testsql.New(t, steps...)
	facts := info.New(db)
	first, err := facts.InitParametersJSON(t.Context(), info.ParameterFilter{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := facts.InitParametersJSON(t.Context(), info.ParameterFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "\"CON_ID\"") || !strings.Contains(string(second), "\"CON_ID\"") || len(state.Calls()) != 5 {
		t.Fatal("stale capability cache")
	}
}
func TestClientReusesIndependentIdentityProperties(t *testing.T) {
	t.Parallel()
	steps := versionSteps("23")
	steps = append(steps, testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"23.0.0.0.0"}}}, testsql.Step{SQL: identity.ConnectedArchitectureSQL, Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}, Values: [][]driver.Value{{"YES", "3", "APP_PDB"}}})
	steps = append(steps, testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"23.0.0.0.0"}}}, testsql.Step{Columns: []string{"EDITION"}, Values: [][]driver.Value{{"SE2"}}})
	db, state := testsql.New(t, steps...)
	facts := info.New(db)
	version, err := facts.Version(t.Context())
	if err != nil || version.Precision != identity.FullPrecision || version.Value != "23.26.1.0.0" {
		t.Fatal(version, err)
	}
	architecture, err := facts.Architecture(t.Context())
	if err != nil || architecture.Value != identity.PDB || architecture.DatabaseCDB.Value != "YES" {
		t.Fatal(architecture, err)
	}
	edition, err := facts.Edition(t.Context())
	if err != nil || edition.Value != identity.StandardEditionFamily || edition.Code.Value != "SE2" {
		t.Fatal(edition, err)
	}
	if len(state.Calls()) != 6 || state.Closed() != 6 {
		t.Fatal("scalar native cursors leaked or shared wiring")
	}
}
func TestClientStopsBeforeParameterSQLOnFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("password=private-sentinel")
	steps := versionSteps("23")
	steps[1].Values = nil
	steps[1].QueryErr = failure
	db, state := testsql.New(t, steps...)
	data, err := info.New(db).InitParametersJSON(t.Context(), info.ParameterFilter{})
	if data != nil || !errors.Is(err, failure) || len(state.Calls()) != 2 || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("failure swallowed, echoed, or query continued")
	}
}
func TestClientRequestGuardsIssueNoSQL(t *testing.T) {
	t.Parallel()
	db, state := testsql.New(t)
	facts := info.New(db)
	if _, err := facts.InitParameters(t.Context(), info.ParameterFilter{Names: []string{""}}); err == nil {
		t.Fatal("bad filter accepted")
	}
	if _, err := facts.SPFileParameters(t.Context(), info.SPFileFilter{SIDs: []string{"x\x00"}}); err == nil {
		t.Fatal("bad SID accepted")
	}
	if _, err := facts.InitParametersJSON(t.Context(), info.ParameterFilter{}, info.JSONLimits{MaxRows: -1}); err == nil {
		t.Fatal("bad limits accepted")
	}
	if _, err := facts.SPFileParametersJSON(t.Context(), info.SPFileFilter{}, info.JSONLimits{}, info.JSONLimits{}); err == nil {
		t.Fatal("multiple limits accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := facts.InitParameters(ctx, info.ParameterFilter{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if len(state.Calls()) != 0 {
		t.Fatal("invalid request queried")
	}
	var nilConn *sql.Conn
	if _, err := info.New(nilConn).Version(t.Context()); err == nil {
		t.Fatal("typed nil native handle accepted")
	}
	var nilClient *info.Client
	if _, err := nilClient.Version(t.Context()); err == nil {
		t.Fatal("nil client accepted")
	}
}
