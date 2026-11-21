package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/asm"
	"github.com/marcuoli/go-oracle-info/dataguard"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-info/passwordfile"
	"github.com/marcuoli/go-oracle-info/redo"
	"github.com/marcuoli/go-oracle-info/storage"
	"github.com/marcuoli/go-oracle-session/rowset"
)

type factQueryer func(context.Context, string, ...any) (*sql.Rows, error)

func (q factQueryer) QueryContext(ctx context.Context, text string, args ...any) (*sql.Rows, error) {
	return q(ctx, text, args...)
}

type factJSONQuery struct {
	name      string
	statement func() (string, []any, error)
	read      func(context.Context, rowset.Queryer, rowset.Limits) (json.RawMessage, error)
	wantSQL   string
	wantArgs  []any
	wantError error
}

func factJSONWithOptions[O any](name string, options O, statement func(O) (string, []any, error),
	read func(context.Context, rowset.Queryer, O, rowset.Limits) (json.RawMessage, error),
	wantSQL string, wantArgs []any, wantError error,
) factJSONQuery {
	return factJSONQuery{
		name:      name,
		statement: func() (string, []any, error) { return statement(options) },
		read: func(ctx context.Context, q rowset.Queryer, limits rowset.Limits) (json.RawMessage, error) {
			return read(ctx, q, options, limits)
		},
		wantSQL: wantSQL, wantArgs: wantArgs, wantError: wantError,
	}
}

func TestFactJSONQueryPreservesCallerAndErrorContract(t *testing.T) {
	ids := []int64{7, 2}
	idBinds := []any{int64(7), int64(2)}
	const major = 19
	passwordOptions := passwordfile.Options{OracleVersion: "23.26.1.0.0", IncludeExascale: true}

	for _, tc := range []factJSONQuery{
		factJSONWithOptions("asm", asm.ClientsOptions{OracleMajor: major, Filter: asm.Filter{SourceRole: asm.DatabaseInstance, Instances: ids}},
			asm.ClientsStatementWithOptions, asm.ClientsJSONWithOptions, "CON_ID AS CON_ID", idBinds, &asm.Error{Source: "GV$ASM_CLIENT"}),
		factJSONWithOptions("data guard", dataguard.ScopeOptions{OracleVersion: "23.26.1.0.0", Filter: dataguard.Filter{Instances: ids}},
			dataguard.ArchiveGapsStatementWithOptions, dataguard.ArchiveGapsJSONWithOptions, "GV$ARCHIVE_GAP", idBinds, &dataguard.Error{Source: "GV$ARCHIVE_GAP"}),
		{
			name: "instance inventory",
			statement: func() (string, []any, error) {
				statement, err := instances.Statement(major)
				return statement, nil, err
			},
			read: func(ctx context.Context, q rowset.Queryer, limits rowset.Limits) (json.RawMessage, error) {
				return instances.JSON(ctx, q, major, limits)
			},
			wantSQL: "GV$INSTANCE", wantError: &instances.Error{Operation: "query"},
		},
		{
			name: "instance versions",
			statement: func() (string, []any, error) {
				statement, err := instances.VersionsStatement(major)
				return statement, nil, err
			},
			read: func(ctx context.Context, q rowset.Queryer, limits rowset.Limits) (json.RawMessage, error) {
				return instances.VersionsJSON(ctx, q, major, limits)
			},
			wantSQL: "VERSION_FULL", wantError: &instances.Error{Operation: "query"},
		},
		factJSONWithOptions("parameters", parameters.SourceOptions{OracleMajor: major, Filter: parameters.Filter{Names: []string{"processes"}, Instances: ids}},
			parameters.EffectiveStatementWithOptions, parameters.EffectiveJSONWithOptions, "GV$SYSTEM_PARAMETER2", []any{int64(7), int64(2), "processes"}, &parameters.Error{Source: "GV$SYSTEM_PARAMETER2"}),
		factJSONWithOptions("hidden context", parameters.HiddenContextOptions{OracleMajor: major, Filter: parameters.HiddenContextFilter{Names: []string{"_example"}, NonDefaultOnly: true}},
			parameters.HiddenContextStatementWithOptions, parameters.HiddenContextJSONWithOptions, "KSPFTCTXDF, 'NLS_SORT=BINARY') <>", []any{"_example"}, &parameters.Error{Source: "SYS.X$KSPPI+SYS.X$KSPPCV2"}),
		{
			name: "password file",
			statement: func() (string, []any, error) {
				statement, err := passwordfile.Statement(passwordOptions)
				return statement, nil, err
			},
			read: func(ctx context.Context, q rowset.Queryer, limits rowset.Limits) (json.RawMessage, error) {
				return passwordfile.JSON(ctx, q, passwordOptions, limits)
			},
			wantSQL: "p.IS_EXC AS IS_EXC", wantError: &passwordfile.Error{Operation: "query"},
		},
		factJSONWithOptions("redo", redo.LogOptions{OracleMajor: major, Filter: redo.Filter{Instances: ids}},
			redo.GroupsStatementWithOptions, redo.GroupsJSONWithOptions, "GV$LOG", idBinds, &redo.Error{Source: "GV$LOG"}),
		factJSONWithOptions("storage", storage.SGAComponentsOptions{OracleMajor: major, Filter: storage.InstanceFilter{Instances: ids}},
			storage.SGAComponentsStatementWithOptions, storage.SGAComponentsJSONWithOptions, "GV$SGASTAT", idBinds, &storage.Error{Source: "GV$SGASTAT"}),
	} {
		t.Run(tc.name, func(t *testing.T) {
			statement, args, err := tc.statement()
			if err != nil || !strings.Contains(statement, tc.wantSQL) || !slices.Equal(args, tc.wantArgs) {
				t.Fatalf("selector query = %q, %v, %v", statement, args, err)
			}
			ctx := t.Context()
			db, state := testsql.New(t, testsql.Step{Columns: []string{"VALUE"}, Values: [][]driver.Value{{"unconsumed"}}})
			cursor, err := db.QueryContext(ctx, "fixture cursor")
			if err != nil {
				t.Fatal(err)
			}
			defer cursor.Close()
			failure := errors.New("private query failure")
			calls := 0
			queryer := factQueryer(func(gotCtx context.Context, gotSQL string, gotArgs ...any) (*sql.Rows, error) {
				calls++
				if gotCtx != ctx || gotSQL != statement || !slices.Equal(gotArgs, tc.wantArgs) {
					t.Fatalf("query context/selection changed, got %q %v", gotSQL, gotArgs)
				}
				return cursor, failure
			})

			if payload, err := tc.read(ctx, queryer, rowset.Limits{MaxRows: -1}); payload != nil || err != rowset.ErrInvalidRequest || calls != 0 {
				t.Fatalf("invalid limits queried or lost precedence: %s, %v, calls=%d", payload, err, calls)
			}
			payload, err := tc.read(ctx, queryer, rowset.Limits{MaxRows: 2, MaxBytes: 32768})
			if payload != nil || reflect.TypeOf(err) != reflect.TypeOf(tc.wantError) || err.Error() != tc.wantError.Error() || !errors.Is(err, failure) {
				t.Fatalf("wrong top-level query error: %s, %T %v; want %T %v with private cause", payload, err, err, tc.wantError, tc.wantError)
			}
			inner, ok := errors.Unwrap(err).(*rowset.Error)
			if !ok || inner.Operation != "query" || !errors.Is(inner, failure) || calls != 1 || state.Closed() != 1 || cursor.Next() {
				t.Fatalf("query layering or close changed: error=%v calls=%d closes=%d", inner, calls, state.Closed())
			}
		})
	}
}

func TestFactJSONAdmissionPrecedesInvalidLimits(t *testing.T) {
	options := parameters.HiddenContextOptions{OracleMajor: 10, Filter: parameters.HiddenContextFilter{Names: []string{"_example"}}}
	q := factQueryer(func(context.Context, string, ...any) (*sql.Rows, error) {
		t.Fatal("unsupported source queried")
		return nil, nil
	})
	payload, err := parameters.HiddenContextJSONWithOptions(t.Context(), q, options, rowset.Limits{MaxRows: -1})
	if payload != nil || err != parameters.ErrUnsupportedVersion {
		t.Fatalf("source admission lost precedence: %s, %v", payload, err)
	}
}
