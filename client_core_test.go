package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/patches"
	"github.com/marcuoli/go-oracle-info/registry"
	"github.com/marcuoli/go-oracle-info/services"
	"github.com/marcuoli/go-oracle-session/rowset"
	"reflect"
	"strings"
	"testing"
)

type coreDataset struct {
	name, source, statement string
	columns                 []string
	values                  []driver.Value
	rows                    func(*info.Client, context.Context) (*sql.Rows, error)
	json                    func(*info.Client, context.Context, info.JSONLimits) (json.RawMessage, error)
}

func coreDatasets(t *testing.T) []coreDataset {
	t.Helper()
	nls, _ := database.NLSDatabaseStatement(19)
	options, _ := database.OptionsStatement(19)
	component, _ := registry.Statement(19)
	service, _ := services.Statement(19)
	dbinfo, _ := identity.DatabaseInfoStatement(identity.DatabaseInfoOptions{OracleMajor: 19, IncludeSupplementalPL: true})
	observed, _ := identity.SessionContextStatement(19)
	invalid, _, _ := objects.InvalidDetailsStatement(19, "Case'Owner")
	history, _ := patches.HistoryStatement(patches.HistoryOptions{OracleMajor: 19, IncludeBundleSeries: true})
	sqlpatch, _ := patches.SQLPatchStatement(patches.SQLPatchOptions{OracleMajor: 19})
	return []coreDataset{
		{"nls", "NLS_DATABASE_PARAMETERS", nls, []string{"NAME", "VALUE"}, []driver.Value{"NLS_NCHAR_CHARACTERSET", " AL16UTF16 "}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.DatabaseNLS(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.DatabaseNLSJSON(ctx, l)
		}},
		{"options", "V$OPTION", options, []string{"PARAMETER", "VALUE"}, []driver.Value{"future option", "UNKNOWN"}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.DatabaseOptions(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.DatabaseOptionsJSON(ctx, l)
		}},
		{"components", "DBA_REGISTRY", component, []string{"COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"}, []driver.Value{"CATALOG", "Dictionary", "19.0.0.0.0", nil, "OPTION OFF", " raw local date "}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.Components(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.ComponentsJSON(ctx, l)
		}},
		{"services", "DBA_SERVICES", service, []string{"NAME", "NETWORK_NAME", "CREATION_DATE", "CREATION_DATE_HASH", "FAILOVER_METHOD", "FAILOVER_TYPE", "FAILOVER_RETRIES", "FAILOVER_DELAY", "MIN_CARDINALITY", "MAX_CARDINALITY", "GOAL", "DTP", "ENABLED", "AQ_HA_NOTIFICATIONS", "CLB_GOAL", "EDITION"}, []driver.Value{"Svc", "Svc.example", "2026-11-15T00:00:00", "4294967295", nil, "future", "12", "30", nil, "0", "SERVICE_TIME", "N", "UNKNOWN", "YES", "LONG", nil}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.Services(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.ServicesJSON(ctx, l)
		}},
		{"database", "V$DATABASE", dbinfo, []string{"DBID", "NAME", "DB_UNIQUE_NAME", "LOG_MODE", "CONTROLFILE_TYPE", "VERSION_TIME", "OPEN_MODE", "DATABASE_ROLE", "SUPPLEMENTAL_LOG_DATA_MIN", "SUPPLEMENTAL_LOG_DATA_PK", "SUPPLEMENTAL_LOG_DATA_UI", "FORCE_LOGGING", "PLATFORM_ID", "PLATFORM_NAME", "CURRENT_SCN", "FLASHBACK_ON", "SUPPLEMENTAL_LOG_DATA_FK", "SUPPLEMENTAL_LOG_DATA_ALL", "SUPPLEMENTAL_LOG_DATA_PL"}, []driver.Value{"4294967295", "DB", "Unique", "ARCHIVELOG", "STANDBY", "2026-11-15T00:00:00", "MOUNTED", "PHYSICAL STANDBY", "IMPLICIT", "YES", "NO", "STANDBY NOLOGGING FOR DATA AVAILABILITY", "13", "Linux x86 64-bit", "9007199254740993", "RESTORE POINT ONLY", "NO", "NO", nil}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.DatabaseInfo(ctx, info.DatabaseInfoOptions{IncludeSupplementalPL: true})
		}, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.DatabaseInfoJSON(ctx, info.DatabaseInfoOptions{IncludeSupplementalPL: true}, l)
		}},
		{"session", "SYS_CONTEXT(USERENV)", observed, []string{"CLIENT_HOST", "OS_USER", "SESSION_USER"}, []driver.Value{"client.example", " OS User ", "SESSION_USER"}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.SessionContext(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.SessionContextJSON(ctx, l)
		}},
		{"invalid", "DBA_OBJECTS", invalid, []string{"OWNER", "OBJECT_TYPE", "OBJECT_NAME", "STATUS", "CREATED", "LAST_DDL_TIME"}, []driver.Value{"Case'Owner", "PACKAGE BODY", "Mixed Name", "INVALID", "2020-01-01T01:02:03", nil}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.InvalidObjects(ctx, "Case'Owner")
		}, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.InvalidObjectsJSON(ctx, []string{"Case'Owner"}, l)
		}},
		{"history", "DBA_REGISTRY_HISTORY", history, []string{"ACTION_TIME", "ACTION", "NAMESPACE", "VERSION", "ID", "COMMENTS", "BUNDLE_SERIES"}, []driver.Value{"2026-11-15T00:00:00.123456", "UPGRADE", "SERVER", "19.0.0.0.0", "123", " multiline\ncomment ", nil}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.PatchHistory(ctx, info.PatchHistoryOptions{IncludeBundleSeries: true})
		}, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.PatchHistoryJSON(ctx, info.PatchHistoryOptions{IncludeBundleSeries: true}, l)
		}},
		{"sqlpatch", "DBA_REGISTRY_SQLPATCH", sqlpatch, []string{"PATCH_ID", "PATCH_UID", "ACTION", "STATUS", "ACTION_TIME", "DESCRIPTION", "LOGFILE", "FLAGS", "INSTALL_ID", "PATCH_TYPE", "SOURCE_VERSION", "SOURCE_BUILD_DESCRIPTION", "SOURCE_BUILD_TIMESTAMP", "TARGET_VERSION", "TARGET_BUILD_DESCRIPTION", "TARGET_BUILD_TIMESTAMP", "RU_LOGFILE"}, []driver.Value{"123", "456", "ROLLBACK", "WITH ERRORS", "2026-11-15T00:00:00.000001", nil, "private-log", "R", "7", "future", "19.1.0.0.0", nil, nil, "19.2.0.0.0", nil, nil, nil}, func(c *info.Client, ctx context.Context) (*sql.Rows, error) { return c.SQLPatches(ctx) }, func(c *info.Client, ctx context.Context, l info.JSONLimits) (json.RawMessage, error) {
			return c.SQLPatchesJSON(ctx, l)
		}},
	}
}

func TestCoreFacadesPreserveNativeCursorAndCompleteJSON(t *testing.T) {
	for _, dataset := range coreDatasets(t) {
		t.Run(dataset.name, func(t *testing.T) {
			step := testsql.Step{SQL: dataset.statement, Columns: dataset.columns, Values: [][]driver.Value{dataset.values}}
			db, state := testsql.New(t, append(versionSteps("19"), step)...)
			spy := &nativeSpy{db: db}
			cursor, err := dataset.rows(info.New(spy), t.Context())
			if err != nil || cursor != spy.last || !cursor.Next() {
				t.Fatalf("native cursor identity/lifetime: %v", err)
			}
			if err = cursor.Close(); err != nil {
				t.Fatal(err)
			}
			if state.Closed() != 3 {
				t.Fatalf("cursor lifecycle: %d closes", state.Closed())
			}
			db2, state2 := testsql.New(t, append(versionSteps("19"), step)...)
			payload, err := dataset.json(info.New(db2), t.Context(), info.JSONLimits{})
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Provenance struct{ Source string }
				Data       rowset.Table
			}
			decoder := json.NewDecoder(strings.NewReader(string(payload)))
			decoder.UseNumber()
			if err = decoder.Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Provenance.Source != dataset.source || len(envelope.Data.Rows) != 1 || len(envelope.Data.Columns) != len(dataset.columns) {
				t.Fatalf("wrong dataset: %s", payload)
			}
			for i, want := range dataset.values {
				got := envelope.Data.Rows[0][i]
				if want == nil {
					if got != nil {
						t.Fatalf("column %s NULL became %v", dataset.columns[i], got)
					}
					continue
				}
				if envelope.Data.Columns[i].Kind == rowset.Integer {
					if got != json.Number(want.(string)) {
						t.Fatalf("exact integer %s became %v", want, got)
					}
				} else if got != want {
					t.Fatalf("raw column %s: got %#v want %#v", dataset.columns[i], got, want)
				}
			}
			if state2.Closed() != 3 {
				t.Fatal("JSON did not finalize its own cursor")
			}
			if dataset.name == "invalid" {
				calls := state2.Calls()
				if len(calls[2].Args) != 1 || calls[2].Args[0].Value != "Case'Owner" || strings.Contains(calls[2].SQL, "Case'Owner") {
					t.Fatal("owner filter was not bound")
				}
			}
		})
	}
}

func TestCoreJSONRejectsEveryFailedPrefix(t *testing.T) {
	failure := errors.New("private failure")
	for _, dataset := range coreDatasets(t) {
		for _, mode := range []string{"terminal", "close", "cancel", "rows", "bytes", "late-scan"} {
			t.Run(dataset.name+"/"+mode, func(t *testing.T) {
				values := append([]driver.Value(nil), dataset.values...)
				step := testsql.Step{SQL: dataset.statement, Columns: dataset.columns, Values: [][]driver.Value{values}}
				limits := info.JSONLimits{}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var want error
				switch mode {
				case "terminal":
					step.NextErr = failure
					want = failure
				case "close":
					step.CloseErr = failure
					want = failure
				case "cancel":
					step.OnNext = func(index int) {
						if index == 1 {
							cancel()
						}
					}
					want = context.Canceled
				case "rows":
					step.Values = append(step.Values, values)
					limits.MaxRows = 1
					want = rowset.ErrCapacity
				case "bytes":
					limits.MaxBytes = 1
					want = rowset.ErrCapacity
				case "late-scan":
					bad := append([]driver.Value(nil), values...)
					bad[0] = "1.5"
					if dataset.name != "sqlpatch" {
						bad[0] = float64(1.5)
					}
					step.Values = append(step.Values, bad)
					want = rowset.ErrInvalidEvidence
				}
				db, state := testsql.New(t, append(versionSteps("19"), step)...)
				payload, err := dataset.json(info.New(db), ctx, limits)
				// Scalar datasets correctly reject a second row as capacity before scanning it.
				if mode == "late-scan" && (dataset.name == "database" || dataset.name == "session") {
					want = rowset.ErrCapacity
				}
				if payload != nil || !errors.Is(err, want) {
					t.Fatalf("published prefix or lost failure: %s %v (want %v)", payload, err, want)
				}
				if mode != "bytes" && state.Closed() != 3 {
					t.Fatalf("failed cursor not closed: %d", state.Closed())
				}
			})
		}
	}
}

func TestSQLPatchFacadeSelectsActualTwelveMinor(t *testing.T) {
	for _, minor := range []int{1, 2} {
		t.Run(string(rune('0'+minor)), func(t *testing.T) {
			version := "12." + string(rune('0'+minor)) + ".0.2.0"
			options := patches.SQLPatchOptions{OracleMajor: 12, OracleMinor: minor}
			statement, err := patches.SQLPatchStatement(options)
			if err != nil {
				t.Fatal(err)
			}
			columns := []string{"PATCH_ID", "PATCH_UID", "ACTION", "STATUS", "ACTION_TIME", "DESCRIPTION", "LOGFILE", "FLAGS"}
			if minor == 2 {
				columns = append(columns, "INSTALL_ID")
			}
			columns = append(columns, "VERSION", "BUNDLE_SERIES", "BUNDLE_ID")
			db, state := testsql.New(t, testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{version}}}, testsql.Step{SQL: statement, Columns: columns})
			payload, err := info.New(db).SQLPatchesJSON(t.Context())
			if err != nil || !json.Valid(payload) || len(state.Calls()) != 2 {
				t.Fatalf("12.%d selection: %s %v", minor, payload, err)
			}
		})
	}
}

func TestCoreJSONInvalidLimitsDoNotQuery(t *testing.T) {
	for _, dataset := range coreDatasets(t) {
		db, state := testsql.New(t)
		payload, err := dataset.json(info.New(db), t.Context(), info.JSONLimits{MaxRows: -1})
		if payload != nil || !errors.Is(err, rowset.ErrInvalidRequest) || !reflect.DeepEqual(state.Calls(), []testsql.Call(nil)) {
			t.Fatalf("%s invalid limit queried: %v", dataset.name, err)
		}
	}
}

func TestServicesFacadeUsesPatchsetEditionBoundary(t *testing.T) {
	for _, version := range []string{"11.1.0.7.0", "11.2.0.1.0", "11.2.0.2.0", "11.2.0.4.0"} {
		t.Run(version, func(t *testing.T) {
			include := version == "11.2.0.2.0" || version == "11.2.0.4.0"
			statement, err := services.Statement(11, include)
			if err != nil {
				t.Fatal(err)
			}
			columns := []string{"NAME", "NETWORK_NAME", "CREATION_DATE", "CREATION_DATE_HASH", "FAILOVER_METHOD", "FAILOVER_TYPE", "FAILOVER_RETRIES", "FAILOVER_DELAY", "MIN_CARDINALITY", "MAX_CARDINALITY", "GOAL", "DTP", "ENABLED", "AQ_HA_NOTIFICATIONS", "CLB_GOAL"}
			if include {
				columns = append(columns, "EDITION")
			}
			db, state := testsql.New(t, testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{version}}}, testsql.Step{SQL: statement, Columns: columns})
			payload, err := info.New(db).ServicesJSON(t.Context())
			if err != nil || !json.Valid(payload) || len(state.Calls()) != 2 {
				t.Fatalf("%s selection: %s %v", version, payload, err)
			}
		})
	}
}
