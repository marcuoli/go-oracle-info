package grants

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestClassifiedGrantReleaseProjections(t *testing.T) {
	for _, tt := range []struct {
		release                                string
		major                                  int
		supported, common, delegate, inherited bool
	}{
		{release: "9.2.0.8.0", major: 9},
		{release: "10.1.0.5.0", major: 10},
		{release: "10.2.0.5.0", major: 10},
		{release: "11.1.0.7.0", major: 11, supported: true},
		{release: "11.2.0.4.0", major: 11, supported: true},
		{major: 12, supported: true, common: true},
		{release: "12.1.0.1.0", major: 12, supported: true, common: true},
		{release: "12.1.0.2.0", major: 12, supported: true, common: true, delegate: true},
		{release: "12.2.0.1.0", major: 12, supported: true, common: true, delegate: true, inherited: true},
		{release: "18.0.0.0.0", major: 18, supported: true, common: true, delegate: true, inherited: true},
		{release: "19.3.0.0.0", major: 19, supported: true, common: true, delegate: true, inherited: true},
		{release: "21.3.0.0.0", major: 21, supported: true, common: true, delegate: true, inherited: true},
		{release: "23.3.0.0.0", major: 23, supported: true, common: true, delegate: true, inherited: true},
		{release: "23.9.0.0.0", major: 23, supported: true, common: true, delegate: true, inherited: true},
		{release: "23.26.1.0.0", major: 23, supported: true, common: true, delegate: true, inherited: true},
		{release: "26.0.0.0.0", major: 26},
	} {
		t.Run(tt.release, func(t *testing.T) {
			options := ClassifiedOptions{Options: Options{OracleMajor: tt.major}, OracleRelease: tt.release}
			for _, source := range []struct {
				table    string
				selector func(ClassifiedOptions) (string, []any, error)
			}{
				{"DBA_TAB_PRIVS", ObjectPrivilegeGrantsStatement},
				{"DBA_COL_PRIVS", ColumnPrivilegeGrantsStatement},
				{"DBA_ROLE_PRIVS", RoleMembershipsStatement},
			} {
				stmt, args, err := source.selector(options)
				if !tt.supported {
					if !errors.Is(err, ErrUnsupportedVersion) || stmt != "" || args != nil {
						t.Fatalf("%s returned a statement for unsupported version: %q, %v, %v", source.table, stmt, args, err)
					}
					continue
				}
				if err != nil || len(args) != 0 {
					t.Fatalf("%s could not select source: %v, %v", source.table, args, err)
				}
				for _, needed := range []string{"FROM " + source.table + " t JOIN", "FROM DBA_USERS", "FROM DBA_ROLES", "p.GRANTEE_KIND AS GRANTEE_KIND", "'USER' AS GRANTEE_KIND", "'ROLE' AS GRANTEE_KIND", "NLSSORT('PUBLIC', 'NLS_SORT=BINARY')"} {
					if !strings.Contains(stmt, needed) {
						t.Errorf("%s omitted %q", source.table, needed)
					}
				}
				for flag, expect := range map[string]bool{
					"t.COMMON AS COMMON":                   tt.common,
					"t.INHERITED AS INHERITED":             tt.inherited,
					"t.HIERARCHY AS HIERARCHY":             source.table == "DBA_TAB_PRIVS",
					"t.DELEGATE_OPTION AS DELEGATE_OPTION": tt.delegate && source.table == "DBA_ROLE_PRIVS",
				} {
					if strings.Contains(stmt, flag) != expect {
						t.Errorf("%s: %s presence should be %t", source.table, flag, expect)
					}
				}
				if source.table == "DBA_TAB_PRIVS" {
					if strings.Contains(stmt, "t.TYPE AS OBJECT_TYPE") != (tt.major >= 12) ||
						strings.Contains(stmt, "CAST(NULL AS VARCHAR2(24)) AS OBJECT_TYPE") != (tt.major == 11) {
						t.Errorf("wrong object type projection: %s", stmt)
					}
				}
			}
		})
	}
}

func TestClassifiedGrantsUseBoundExactScope(t *testing.T) {
	options := ClassifiedOptions{Options: Options{OracleMajor: 19, Owners: []string{"App"}, Names: []string{"Mixed'Name", "READERS"}}}
	stmt, args, err := ColumnPrivilegeGrantsStatement(options)
	if err != nil || !reflect.DeepEqual(args, []any{"App", "Mixed'Name", "READERS"}) || strings.Contains(stmt, "Mixed'Name") {
		t.Fatalf("unexpected bound column scope: %q, %v, %v", stmt, args, err)
	}
	for _, needed := range []string{
		"NLSSORT(t.OWNER, 'NLS_SORT=BINARY') IN (NLSSORT(:1, 'NLS_SORT=BINARY'))",
		"NLSSORT(t.GRANTEE, 'NLS_SORT=BINARY') IN (NLSSORT(:2, 'NLS_SORT=BINARY'),NLSSORT(:3, 'NLS_SORT=BINARY'))",
		"COUNT(DISTINCT o.OBJECT_TYPE) = 1 THEN MIN(o.OBJECT_TYPE) END",
		"NLSSORT(o.OWNER, 'NLS_SORT=BINARY') = NLSSORT(t.OWNER, 'NLS_SORT=BINARY')",
		"NLSSORT(o.OBJECT_NAME, 'NLS_SORT=BINARY') = NLSSORT(t.TABLE_NAME, 'NLS_SORT=BINARY')",
		"o.SUBOBJECT_NAME IS NULL",
		"o.OBJECT_TYPE IN ('TABLE', 'VIEW', 'MATERIALIZED VIEW')",
	} {
		if !strings.Contains(stmt, needed) {
			t.Errorf("statement omitted %q", needed)
		}
	}
	if _, _, err := RoleMembershipsStatement(options); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("membership accepted an owner restriction: %v", err)
	}
	options.Owners = nil
	options.OracleRelease = "12.1.0.2.0"
	if _, _, err := RoleMembershipsStatement(options); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("membership accepted conflicting major evidence: %v", err)
	}
	options.OracleRelease = "19.0.+2.0.0"
	if _, _, err := RoleMembershipsStatement(options); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("membership accepted malformed numeric release: %v", err)
	}
}

func TestClassifiedGrantDecodersRetainRawValues(t *testing.T) {
	options := ClassifiedOptions{Options: Options{OracleMajor: 19, Names: []string{"AppUser", "APP_ROLE"}}}
	objectPlan, err := objectPrivilegeGrantsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"OWNER": "MixedCase", "GRANTOR": "Granter", "GRANTEE": "AppUser", "GRANTEE_KIND": "USER",
		"OBJECT_TYPE": "PACKAGE", "TABLE_NAME": "Pkg", "PRIVILEGE": "EXECUTE",
		"GRANTABLE": "NO", "HIERARCHY": "NO", "COMMON": "YES",
	}
	object := classifiedFixture(objectPlan, values, values)
	objectFacts, err := DecodeObjectPrivilegeGrants(object, options)
	if err != nil || len(objectFacts.Records) != 2 || objectFacts.Records[0].Owner.Text != "MixedCase" ||
		objectFacts.Records[0].GranteeKind.Text != "USER" || objectFacts.Records[0].ObjectType.Text != "PACKAGE" ||
		objectFacts.Records[1].Common == nil || objectFacts.Records[1].Common.Text != "YES" ||
		objectFacts.Records[1].Inherited == nil || !objectFacts.Records[1].Inherited.Null {
		t.Fatalf("object evidence changed: %+v, %v", objectFacts, err)
	}

	columnPlan, err := columnPrivilegeGrantsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	values["GRANTEE"], values["GRANTEE_KIND"], values["COLUMN_NAME"] = "APP_ROLE", "ROLE", "MixedColumn"
	delete(values, "OBJECT_TYPE")
	column := classifiedFixture(columnPlan, values)
	columnFacts, err := DecodeColumnPrivilegeGrants(column, options)
	if err != nil || len(columnFacts.Records) != 1 || columnFacts.Records[0].ColumnName.Text != "MixedColumn" ||
		columnFacts.Records[0].GranteeKind.Text != "ROLE" || !columnFacts.Records[0].ObjectType.Null ||
		columnFacts.Provenance.Source != "DBA_COL_PRIVS+DBA_USERS+DBA_ROLES+DBA_OBJECTS" {
		t.Fatalf("column evidence changed: %+v, %v", columnFacts, err)
	}

	options.OracleMajor = 12
	options.OracleRelease = "12.1.0.2.0"
	rolePlan, err := roleMembershipsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	role := classifiedFixture(rolePlan, map[string]string{
		"GRANTEE": "APP_ROLE", "GRANTEE_KIND": "ROLE", "GRANTED_ROLE": "PARENT_ROLE",
		"ADMIN_OPTION": "NO", "DEFAULT_ROLE": "YES", "COMMON": "NO",
	})
	roleFacts, err := DecodeRoleMemberships(role, options)
	if err != nil || len(roleFacts.Records) != 1 || roleFacts.Records[0].Grantee.Text != "APP_ROLE" ||
		roleFacts.Records[0].GrantedRole.Text != "PARENT_ROLE" || roleFacts.Records[0].DefaultRole.Text != "YES" ||
		roleFacts.Records[0].DelegateOption == nil || !roleFacts.Records[0].DelegateOption.Null ||
		roleFacts.Records[0].Inherited != nil {
		t.Fatalf("role edge or NULL/absence semantics changed: %+v, %v", roleFacts, err)
	}
}

func TestClassifiedMembershipReadAndJSONRejectInvalidObservation(t *testing.T) {
	ref, err := reference.NewReference("unit", "facts", "role-memberships", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Options: Options{OracleMajor: 12}, Reference: ref}
	options := ClassifiedOptions{Options: request.Options, OracleRelease: "12.2.0.1.0"}
	plan, err := roleMembershipsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	good := map[string]string{"GRANTEE": "TestUser", "GRANTEE_KIND": "USER", "GRANTED_ROLE": "APP_READ", "DELEGATE_OPTION": "NO"}
	result := classifiedFixture(plan, good)
	reader := fixtureReader(func(_ context.Context, stmt session.Statement) (session.Result, error) {
		if stmt.Reference != ref || stmt.Text != plan.sql {
			t.Fatalf("unexpected statement or reference: %v", stmt.Reference)
		}
		return result, nil
	})
	facts, err := ReadRoleMembershipsForRelease(t.Context(), reader, request, options.OracleRelease)
	if err != nil || len(facts.Records) != 1 || facts.Records[0].DelegateOption == nil ||
		facts.Records[0].Inherited == nil || !facts.Records[0].Inherited.Null ||
		facts.Provenance.Scope != "connected_container_dba_dictionary" {
		t.Fatalf("read did not preserve the exact projection: %+v, %v", facts, err)
	}
	goodRow := make([]driver.Value, len(result.Columns))
	for i, value := range result.Rows[0] {
		if !value.Null {
			goodRow[i] = value.Text
		}
	}
	db, state := testsql.New(t, testsql.Step{SQL: plan.sql, Columns: result.Columns, Values: [][]driver.Value{goodRow}})
	payload, err := RoleMembershipsJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Data struct {
			Columns []rowset.Column
			Rows    [][]*string
		}
	}
	jsonErr := json.Unmarshal(payload, &envelope)
	data := envelope.Data
	if err != nil || jsonErr != nil || state.Closed() != 1 || len(data.Columns) != 8 ||
		len(data.Rows) != 1 || len(data.Rows[0]) != 8 || data.Columns[7].Name != "INHERITED" || data.Rows[0][7] != nil {
		t.Fatalf("JSON failed to publish complete native rows: %s, %v", payload, err)
	}

	for _, invalid := range []map[string]string{
		{"GRANTEE": "PUBLIC", "GRANTEE_KIND": "ROLE"},
		{"GRANTEE": "TestUser", "GRANTEE_KIND": "OTHER"},
		{"GRANTEE_KIND": "USER"},
	} {
		result = classifiedFixture(plan, good, invalid)
		facts, err := ReadRoleMembershipsForRelease(t.Context(), reader, request, options.OracleRelease)
		if !errors.Is(err, ErrInvalidEvidence) || facts.Records != nil || facts.Provenance.Source != "" {
			t.Fatalf("read published a valid prefix: %+v, %v", facts, err)
		}
	}
	result = classifiedFixture(plan, good)
	cancelled, cancel := context.WithCancel(t.Context())
	cancelling := fixtureReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return result, nil
	})
	facts, err = ReadRoleMembershipsForRelease(cancelled, cancelling, request, options.OracleRelease)
	if !errors.Is(err, context.Canceled) || facts.Records != nil {
		t.Fatalf("read published after cancellation: %+v, %v", facts, err)
	}

	invalidRow := append([]driver.Value(nil), goodRow...)
	invalidRow[1] = "OTHER"
	db, state = testsql.New(t, testsql.Step{SQL: plan.sql, Columns: result.Columns, Values: [][]driver.Value{goodRow, invalidRow}})
	payload, err = RoleMembershipsJSON(t.Context(), db, options, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || payload != nil || state.Closed() != 1 {
		t.Fatalf("invalid native row yielded JSON: %s, %v", payload, err)
	}
}

func classifiedFixture(plan selection, rows ...map[string]string) session.Result {
	result := session.Result{Columns: make([]string, len(plan.schema)), Rows: make([][]session.Value, len(rows))}
	for i := range result.Rows {
		result.Rows[i] = make([]session.Value, len(plan.schema))
	}
	for i, column := range plan.schema {
		result.Columns[i] = column.Name
		for j, row := range rows {
			value, found := row[column.Name]
			result.Rows[j][i] = session.Value{Text: value, Null: !found}
		}
	}
	return result
}
