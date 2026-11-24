package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/containers"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestRootDictionaryReleaseMatrix(t *testing.T) {
	cases := []struct {
		version                  string
		local, root, patch, full bool
	}{
		{"9.2.0.8.0", true, false, false, false},
		{"10.1.0.5.0", true, false, false, false},
		{"10.2.0.5.0", true, false, false, false},
		{"11.1.0.7.0", true, false, false, false},
		{"11.2.0.4.0", true, false, false, false},
		{"12.1", true, false, false, false},
		{"12.1.0.1.0", true, true, false, false},
		{"12.1.0.2.0", true, true, true, false},
		{"12.2.0.1.0", true, true, true, false},
		{"18.1.0.0.0", true, true, true, true},
		{"19.33.0.0.0", true, true, true, true},
		{"21.21.0.0.0", true, true, true, true},
		{"23.9.0.0.0", true, true, true, true},
		{"23.26.1.0.0", true, true, true, true},
		{"23.26.4.0.0", true, true, true, true},
		{"8.1.7.4.0", false, false, false, false},
		{"26ai", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			global, err := identity.GlobalNameStatement(identity.GlobalNameOptions{OracleVersion: tc.version})
			if tc.local {
				if err != nil || !strings.Contains(global, " FROM SYS.GLOBAL_NAME v") || strings.Contains(global, " AS SOURCE_CON_ID") != (tc.root || tc.version == "12.1") {
					t.Fatalf("global projection %q %v", global, err)
				}
			} else if global != "" || !errors.Is(err, identity.ErrUnsupportedVersion) {
				t.Fatalf("unsupported name scope %q %v", global, err)
			}
			props, err := database.PropertiesStatement(database.PropertiesOptions{OracleVersion: tc.version})
			if tc.local {
				if err != nil || !strings.Contains(props, " FROM SYS.DATABASE_PROPERTIES v") || strings.Contains(props, " AS SOURCE_CON_ID") != (tc.root || tc.version == "12.1") {
					t.Fatalf("property projection %q %v", props, err)
				}
			} else if props != "" || !errors.Is(err, database.ErrUnsupportedVersion) {
				t.Fatalf("unsupported property scope %q %v", props, err)
			}
			opt := containers.RootDictionaryOptions{Scope: containers.Root, OracleVersion: tc.version}
			for _, v := range []struct {
				view      string
				supported bool
				statement func(containers.RootDictionaryOptions) (string, error)
			}{
				{"CDB_REGISTRY", tc.root, containers.RegistryComponentsStatement},
				{"CDB_OBJECTS", tc.root, containers.InvalidObjectCountsStatement},
				{"CDB_REGISTRY_SQLPATCH", tc.patch, containers.SQLPatchHistoryStatement},
				{"CDB_TABLES", tc.root, containers.LegacyAuditTablePlacementStatement},
			} {
				sql, err := v.statement(opt)
				if !v.supported {
					if sql != "" || !errors.Is(err, containers.ErrUnsupportedVersion) {
						t.Fatalf("%s unsupported version selected SQL: %q %v", v.view, sql, err)
					}
					continue
				}
				if err != nil || !strings.Contains(sql, " FROM SYS."+v.view+" v WHERE TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) = 1") {
					t.Fatalf("%s source scope: %q %v", v.view, sql, err)
				}
				if (v.view == "CDB_REGISTRY" && tc.full) != strings.Contains(sql, " AS VERSION_FULL") {
					t.Fatalf("incorrect full version column: %s", sql)
				}
				opt.Scope = containers.CurrentContainer
				if sql, err := v.statement(opt); sql != "" || !errors.Is(err, containers.ErrUnsupportedScope) {
					t.Fatalf("non-root scope selected SQL: %q %v", sql, err)
				}
				opt.Scope = containers.Root
			}
		})
	}
}

func TestRootDictionaryExactVisibleEvidence(t *testing.T) {
	root := containers.RootDictionaryOptions{OracleVersion: "23.26.1.0.0", Scope: containers.Root}
	txt := func(s string) session.Value { return session.Value{Text: s} }
	null := session.Value{Null: true}
	const large = "900719925474099312345"
	sql, err := containers.InvalidObjectCountsStatement(root)
	if err != nil || !strings.Contains(sql, "COUNT(CASE WHEN v.STATUS = 'INVALID' THEN 1 END)") || !strings.Contains(sql, " GROUP BY v.CON_ID") {
		t.Fatalf("visible-group count contract: %s %v", sql, err)
	}
	cols := []string{"CON_ID", "INVALID_COUNT"}
	result := session.Result{Columns: cols, Rows: [][]session.Value{{txt("1"), txt(large)}, {txt("3"), txt("0")}}}
	decoded, err := containers.DecodeInvalidObjectCounts(result, root)
	if err != nil || decoded.Provenance.Complete || len(decoded.Rows) != 2 || decoded.Rows[0].Count.Text != large || decoded.Rows[1].Count.Text != "0" || decoded.Rows[1].ContainerID.Text != "3" {
		t.Fatalf("retained exact groups: %+v %v", decoded, err)
	}
	result.Rows = append(result.Rows, []session.Value{txt("03"), txt("1")})
	decoded, err = containers.DecodeInvalidObjectCounts(result, root)
	if decoded.Rows != nil || !errors.Is(err, containers.ErrInvalidEvidence) {
		t.Fatalf("duplicate numeric groups were accepted: %+v %v", decoded, err)
	}
	empty, err := containers.DecodeInvalidObjectCounts(session.Result{Columns: cols}, root)
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("successful empty must remain separate: %+v %v", empty, err)
	}

	patchCols := []string{"CON_ID", "PATCH_ID", "PATCH_UID", "ACTION", "STATUS", "ACTION_TIME", "DESCRIPTION"}
	patch := []session.Value{txt("3"), txt(large), txt("42"), txt("ROLLBACK"), txt("WITH ERRORS"), txt("2026-11-24T09:08:07.123456"), null}
	answer, err := containers.DecodeSQLPatchHistory(session.Result{Columns: patchCols, Rows: [][]session.Value{patch, patch}}, root)
	if err != nil || len(answer.Rows) != 2 || answer.Rows[1].PatchID.Text != large || answer.Rows[1].Status.Text != "WITH ERRORS" || !answer.Rows[1].Description.Null {
		t.Fatalf("patch attempts lost source values or duplicates: %+v %v", answer, err)
	}
	malformed := append([]session.Value(nil), patch...)
	malformed[5] = txt("2026-11-24T09:08:07.123456Z")
	if got, err := containers.DecodeSQLPatchHistory(session.Result{Columns: patchCols, Rows: [][]session.Value{patch, malformed}}, root); got.Rows != nil || !errors.Is(err, containers.ErrInvalidEvidence) {
		t.Fatalf("malformed late patch date leaked prefix: %+v %v", got, err)
	}

	auditCols := []string{"CON_ID", "OWNER", "TABLE_NAME", "TABLESPACE_NAME"}
	audit, err := containers.DecodeLegacyAuditTablePlacement(session.Result{Columns: auditCols, Rows: [][]session.Value{{txt("3"), txt("SYS"), txt("AUD$"), null}}}, root)
	if err != nil || len(audit.Rows) != 1 || audit.Rows[0].ContainerID.Text != "3" || !audit.Rows[0].TablespaceName.Null {
		t.Fatalf("legacy audit placement: %+v %v", audit, err)
	}
	if audit, err = containers.DecodeLegacyAuditTablePlacement(session.Result{Columns: auditCols, Rows: [][]session.Value{{txt("3"), txt("AUDSYS"), txt("AUD$UNIFIED"), txt("SYSAUX")}}}, root); audit.Rows != nil || !errors.Is(err, containers.ErrInvalidEvidence) {
		t.Fatalf("unadmitted audit object returned: %+v %v", audit, err)
	}
}

func TestRootDictionaryAdmissionAndCompletion(t *testing.T) {
	ctx := t.Context()
	root := containers.RootDictionaryOptions{OracleVersion: "23.26.1.0.0", Scope: containers.Root}
	request := containers.RegistryComponentsRequest{Options: root, Limits: rowset.Limits{MaxRows: 1}}
	ref, err := reference.NewReference("go-oracle-info", "tests", "root-dictionary", 1)
	if err != nil {
		t.Fatal(err)
	}
	request.Reference = ref
	columns := []string{"CON_ID", "COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"}
	row := []session.Value{{Text: "3"}, {Text: "CATALOG"}, {Text: "Catalog"}, {Text: "23"}, {Null: true}, {Text: "VALID"}, {Null: true}}
	reader := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
		return session.Result{Columns: columns, Rows: [][]session.Value{row, row}}, nil
	})
	facts, err := containers.ReadRegistryComponents(ctx, reader, request)
	if facts.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("buffer limit produced incomplete output: %+v %v", facts, err)
	}
	request.Options.Scope = containers.CurrentContainer
	unopened := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
		t.Fatal("attempted query on unsupported connected PDB")
		return session.Result{}, nil
	})
	if result, err := containers.ReadRegistryComponents(ctx, unopened, request); result.Rows != nil || !errors.Is(err, containers.ErrUnsupportedScope) {
		t.Fatalf("unsupported scope result: %+v %v", result, err)
	}
	stmt, err := containers.RegistryComponentsStatement(root)
	if err != nil {
		t.Fatal(err)
	}
	terminal := errors.New("cursor tail")
	db, spy := testsql.New(t, testsql.Step{SQL: stmt, Columns: columns, Values: [][]driver.Value{{int64(3), "CATALOG", "Catalog", "23", nil, "VALID", nil}}, NextErr: terminal})
	if blob, err := containers.RegistryComponentsJSON(ctx, db, root, rowset.Limits{}); blob != nil || !errors.Is(err, terminal) || spy.Closed() != 1 {
		t.Fatalf("partial native cursor was published: %s %v", blob, err)
	}
	denied := errors.New("ORA-01031: insufficient privileges")
	request.Options = root
	rejected := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) { return session.Result{}, denied })
	var sourceErr *containers.ReadError
	if facts, err := containers.ReadRegistryComponents(ctx, rejected, request); facts.Rows != nil || !errors.Is(err, denied) || !errors.As(err, &sourceErr) || sourceErr.Outcome != containers.Inaccessible {
		t.Fatalf("access failure became successful empty or lost cause: %+v %v", facts, err)
	}
}

func TestConnectedDictionaryNullableProjection(t *testing.T) {
	modern := identity.GlobalNameOptions{OracleVersion: "23.26.1.0.0"}
	columns := []string{"GLOBAL_NAME", "SOURCE_CON_ID"}
	row := []session.Value{{Null: true}, {Text: "3"}}
	facts, err := identity.DecodeGlobalName(session.Result{Columns: columns, Rows: [][]session.Value{row}}, modern)
	if err != nil || len(facts.Rows) != 1 || !facts.Rows[0].Name.Null || facts.Rows[0].SourceContainerID == nil || facts.Rows[0].SourceContainerID.Text != "3" {
		t.Fatalf("nullable global name %+v %v", facts, err)
	}
	if facts, err = identity.DecodeGlobalName(session.Result{Columns: columns, Rows: [][]session.Value{row, row}}, modern); facts.Rows != nil || !errors.Is(err, identity.ErrInvalidEvidence) {
		t.Fatalf("global name scalar overflow %+v %v", facts, err)
	}
	options := database.PropertiesOptions{OracleVersion: "11.2.0.4.0"}
	sql, err := database.PropertiesStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := testsql.New(t, testsql.Step{SQL: sql, Columns: []string{"PROPERTY_NAME", "PROPERTY_VALUE", "DESCRIPTION"}, Values: [][]driver.Value{{"PROP", nil, "label"}}})
	raw, err := database.PropertiesJSON(t.Context(), db, options, rowset.Limits{})
	var data struct {
		Provenance database.DatasetProvenance
		Data       struct{ Rows [][]json.RawMessage }
	}
	if err != nil || json.Unmarshal(raw, &data) != nil || !data.Provenance.Complete || len(data.Data.Rows) != 1 ||
		len(data.Data.Rows[0]) != 3 || string(data.Data.Rows[0][1]) != "null" {
		t.Fatalf("nullable legacy property result %s %v", raw, err)
	}
}
func TestRootDictionaryRequiredSourceValues(t *testing.T) {
	root := containers.RootDictionaryOptions{OracleVersion: "23.26.1.0.0", Scope: containers.Root}
	columns := []string{"CON_ID", "PATCH_ID", "PATCH_UID", "ACTION", "STATUS", "ACTION_TIME", "DESCRIPTION"}
	valid := []session.Value{
		{Text: "3"}, {Text: "17"}, {Text: "21"}, {Text: "APPLY"}, {Text: "SUCCESS"},
		{Text: "2026-11-24T13:00:00.123456"}, {Null: true},
	}
	for _, index := range []int{1, 2, 3, 4, 5} {
		row := append([]session.Value(nil), valid...)
		row[index] = session.Value{Null: true}
		facts, err := containers.DecodeSQLPatchHistory(session.Result{Columns: columns, Rows: [][]session.Value{valid, row}}, root)
		if facts.Rows != nil || !errors.Is(err, containers.ErrInvalidEvidence) {
			t.Errorf("required %s accepted SQL NULL: %+v %v", columns[index], facts, err)
		}
	}
	legacy := root
	legacy.OracleVersion = "12.1.0.2.0"
	legacyRow := append([]session.Value(nil), valid...)
	legacyRow[4] = session.Value{Null: true}
	facts, err := containers.DecodeSQLPatchHistory(session.Result{Columns: columns, Rows: [][]session.Value{legacyRow}}, legacy)
	if err != nil || len(facts.Rows) != 1 || !facts.Rows[0].Status.Null {
		t.Errorf("legacy nullable status was lost: %+v %v", facts, err)
	}

	components, err := containers.DecodeRegistryComponents(session.Result{
		Columns: []string{"CON_ID", "COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"},
		Rows:    [][]session.Value{{{Text: "1"}, {Null: true}, {Text: "Catalog"}, {Text: "23"}, {Null: true}, {Text: "VALID"}, {Null: true}}},
	}, root)
	if components.Rows != nil || !errors.Is(err, containers.ErrInvalidEvidence) {
		t.Errorf("required COMP_ID accepted SQL NULL: %+v %v", components, err)
	}
}
