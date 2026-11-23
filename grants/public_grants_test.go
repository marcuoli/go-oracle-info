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

func TestExplicitPublicGrantsKeepExactScope(t *testing.T) {
	options := ClassifiedOptions{Options: Options{OracleMajor: 19, Names: []string{"PUBLIC", "Public"}}}
	for _, source := range []struct {
		name string
		plan func(ClassifiedOptions) (selection, error)
	}{
		{"object", objectPrivilegeGrantsPlan},
		{"column", columnPrivilegeGrantsPlan},
		{"role", roleMembershipsPlan},
	} {
		t.Run(source.name, func(t *testing.T) {
			plan, err := source.plan(options)
			if err != nil || !reflect.DeepEqual(plan.args, []any{"PUBLIC", "Public"}) {
				t.Fatalf("unexpected scope: %v, %v", plan.args, err)
			}
			for _, part := range []string{
				"SELECT 'PUBLIC' AS PRINCIPAL_NAME, 'PUBLIC' AS GRANTEE_KIND FROM DUAL",
				"(" + notPublicGrantee + " OR p.GRANTEE_KIND = 'PUBLIC')",
				"NLSSORT(t.GRANTEE, 'NLS_SORT=BINARY') IN (NLSSORT(:1, 'NLS_SORT=BINARY'),NLSSORT(:2, 'NLS_SORT=BINARY'))",
			} {
				if !strings.Contains(plan.sql, part) {
					t.Errorf("missing explicit scope %q", part)
				}
			}
			goodPublic := map[string]string{"GRANTEE": "PUBLIC", "GRANTEE_KIND": "PUBLIC"}
			goodNamed := map[string]string{"GRANTEE": "Public", "GRANTEE_KIND": "ROLE"}
			if _, err := columns(classifiedFixture(plan, goodPublic, goodNamed), plan); err != nil {
				t.Fatalf("valid explicit and named identities did not decode: %v", err)
			}
			for _, invalid := range []map[string]string{
				{"GRANTEE": "PUBLIC", "GRANTEE_KIND": "ROLE"},
				{"GRANTEE": "Public", "GRANTEE_KIND": "PUBLIC"},
				{"GRANTEE": "OTHER", "GRANTEE_KIND": "ROLE"},
			} {
				if _, err := columns(classifiedFixture(plan, goodPublic, invalid), plan); !errors.Is(err, ErrInvalidEvidence) {
					t.Errorf("decoder accepted an invalid grantee: %v, %v", invalid, err)
				}
			}
			for _, names := range [][]string{nil, {"Public"}} {
				named := options
				named.Names = names
				namedPlan, err := source.plan(named)
				if err != nil || strings.Contains(namedPlan.sql, "FROM DUAL") {
					t.Fatalf("ordinary named selection changed: %s, %v", namedPlan.sql, err)
				}
				if _, err := columns(classifiedFixture(namedPlan, goodPublic), namedPlan); !errors.Is(err, ErrInvalidEvidence) {
					t.Errorf("implicit public selection accepted a row: %v", err)
				}
			}
		})
	}
}

func TestPublicRoleMaintenanceVersionBoundary(t *testing.T) {
	for _, tc := range []struct {
		major     int
		version   string
		supported bool
		hasFlag   bool
	}{
		{9, "9.2.0.8.0", false, false},
		{10, "10.1.0.5.0", false, false},
		{10, "10.2.0.5.0", false, false},
		{11, "11.1.0.7.0", true, false},
		{11, "11.2.0.4.0", true, false},
		{12, "", true, false},
		{12, "12.1.0.1.0", true, false},
		{12, "12.1.0.2.0", true, true},
		{12, "12.2.0.1.0", true, true},
		{18, "18.0.0.0.0", true, true},
		{19, "19.32.0.0.0", true, true},
		{21, "21.3.0.0.0", true, true},
		{23, "23.3.0.0.0", true, true},
		{23, "23.9.0.0.0", true, true},
		{23, "23.26.1.0.0", true, true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			options := ClassifiedOptions{Options: Options{OracleMajor: tc.major, Names: []string{"PUBLIC"}}, OracleRelease: tc.version}
			plan, err := roleMembershipsPlan(options)
			if !tc.supported {
				if !errors.Is(err, ErrUnsupportedVersion) || plan.sql != "" {
					t.Fatalf("unsupported release selected SQL: %q, %v", plan.sql, err)
				}
				return
			}
			if err != nil || strings.Contains(plan.sql, "r.ORACLE_MAINTAINED") != tc.hasFlag {
				t.Fatalf("unexpected maintenance projection: %s, %v", plan.sql, err)
			}
			row := map[string]string{"GRANTEE": "PUBLIC", "GRANTEE_KIND": "PUBLIC", "GRANTED_ROLE": "APP_ROLE"}
			facts, err := DecodeRoleMemberships(classifiedFixture(plan, row), options)
			if err != nil || len(facts.Records) != 1 {
				t.Fatalf("unexpected decoded role: %+v, %v", facts, err)
			}
			flag := facts.Records[0].GrantedRoleOracleMaintained
			if tc.hasFlag != (flag != nil) || flag != nil && !flag.Null {
				t.Errorf("did not preserve omitted vs SQL NULL: %+v", flag)
			}
		})
	}
}

func TestPublicRoleReaderAndJSONUseSameProjection(t *testing.T) {
	ref, err := reference.NewReference("unit", "facts", "public-role-membership", 1)
	if err != nil {
		t.Fatal(err)
	}
	options := ClassifiedOptions{Options: Options{OracleMajor: 19, Names: []string{"PUBLIC"}}, OracleRelease: "19.32.0.0.0"}
	plan, err := roleMembershipsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	result := classifiedFixture(plan, map[string]string{
		"GRANTEE": "PUBLIC", "GRANTEE_KIND": "PUBLIC", "GRANTED_ROLE": "APP_ROLE",
		"GRANTED_ROLE_ORACLE_MAINTAINED": "N",
	})
	reader := fixtureReader(func(_ context.Context, stmt session.Statement) (session.Result, error) {
		if stmt.Reference != ref || stmt.Text != plan.sql || !reflect.DeepEqual(stmt.Args, []any{"PUBLIC"}) {
			t.Fatalf("unexpected public grant request: %+v", stmt)
		}
		return result, nil
	})
	facts, err := ReadRoleMembershipsForRelease(t.Context(), reader, Request{Options: options.Options, Reference: ref}, options.OracleRelease)
	if err != nil || len(facts.Records) != 1 || facts.Records[0].GrantedRoleOracleMaintained == nil ||
		facts.Records[0].GrantedRoleOracleMaintained.Text != "N" || !reflect.DeepEqual(facts.Provenance.Names, []string{"PUBLIC"}) {
		t.Fatalf("missing recorded role flag or scope: %+v, %v", facts, err)
	}

	values := make([]driver.Value, len(result.Columns))
	flagPosition := -1
	for i, value := range result.Rows[0] {
		if !value.Null {
			values[i] = value.Text
		}
		if result.Columns[i] == "GRANTED_ROLE_ORACLE_MAINTAINED" {
			flagPosition = i
		}
	}
	if flagPosition < 0 {
		t.Fatal("role flag absent from test input")
	}
	db, state := testsql.New(t, testsql.Step{SQL: plan.sql, Columns: result.Columns, Values: [][]driver.Value{values}})
	payload, err := RoleMembershipsJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Data struct {
			Columns []rowset.Column
			Rows    [][]*string
		}
	}
	jsonErr := json.Unmarshal(payload, &envelope)
	data := envelope.Data
	if err != nil || jsonErr != nil || state.Closed() != 1 || len(data.Columns) != len(result.Columns) ||
		len(data.Rows) != 1 || len(data.Rows[0]) != len(result.Columns) ||
		data.Columns[flagPosition].Name != "GRANTED_ROLE_ORACLE_MAINTAINED" ||
		data.Rows[0][flagPosition] == nil || *data.Rows[0][flagPosition] != "N" {
		t.Fatalf("role JSON does not retain the public projection: %s, %v, %v", payload, err, jsonErr)
	}
}

func TestApplicationPublicGrantsAreFilteredBeforeTransport(t *testing.T) {
	options := ClassifiedOptions{
		Options:               Options{OracleMajor: 19, Names: []string{"PUBLIC"}},
		OracleRelease:         "19.32.0.0.0",
		ApplicationPublicOnly: true,
	}
	for _, source := range []struct {
		name, filter string
		plan         func(ClassifiedOptions) (selection, error)
	}{
		{"object", "AND u.ORACLE_MAINTAINED = 'N'", objectPrivilegeGrantsPlan},
		{"column", "AND u.ORACLE_MAINTAINED = 'N'", columnPrivilegeGrantsPlan},
		{"role", grantedRoleMaintenance + " = 'N'", roleMembershipsPlan},
	} {
		t.Run(source.name, func(t *testing.T) {
			plan, err := source.plan(options)
			if err != nil || plan.provenance.Scope != applicationPublicScope || !strings.Contains(plan.sql, source.filter) {
				t.Fatalf("expected application-only PUBLIC selection: %s, %v", plan.sql, err)
			}
			for _, names := range [][]string{nil, {"Public"}, {"PUBLIC", "APP"}} {
				invalid := options
				invalid.Names = names
				badPlan, err := source.plan(invalid)
				if !errors.Is(err, ErrInvalidRequest) || badPlan.sql != "" {
					t.Errorf("unexpected scope for %q: %s, %v", names, badPlan.sql, err)
				}
			}
			legacy := options
			legacy.OracleMajor, legacy.OracleRelease = 12, "12.1.0.1.0"
			badPlan, err := source.plan(legacy)
			if !errors.Is(err, ErrApplicationPublicUnavailable) || badPlan.sql != "" {
				t.Errorf("application classification selected without qualified metadata: %s, %v", badPlan.sql, err)
			}
		})
	}
	ref, err := reference.NewReference("unit", "facts", "application-public-roles", 1)
	if err != nil {
		t.Fatal(err)
	}
	rolePlan, err := roleMembershipsPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	roleRow := map[string]string{
		"GRANTEE": "PUBLIC", "GRANTEE_KIND": "PUBLIC", "GRANTED_ROLE": "APP_ROLE",
		"GRANTED_ROLE_ORACLE_MAINTAINED": "N",
	}
	reader := fixtureReader(func(_ context.Context, stmt session.Statement) (session.Result, error) {
		if stmt.Reference != ref || stmt.Text != rolePlan.sql || !reflect.DeepEqual(stmt.Args, []any{"PUBLIC"}) {
			t.Fatalf("unexpected application PUBLIC statement or scope: %+v", stmt)
		}
		return classifiedFixture(rolePlan, roleRow), nil
	})
	request := ClassifiedRequest{Options: options, Reference: ref}
	facts, err := ReadRoleMembershipsWithOptions(t.Context(), reader, request)
	if err != nil || len(facts.Records) != 1 || facts.Provenance.Scope != applicationPublicScope || facts.Records[0].GrantedRole.Text != "APP_ROLE" {
		t.Fatalf("missing positively classified PUBLIC grant: %+v, %v", facts, err)
	}
	roleRow["GRANTED_ROLE_ORACLE_MAINTAINED"] = "Y"
	facts, err = ReadRoleMembershipsWithOptions(t.Context(), reader, request)
	if !errors.Is(err, ErrInvalidEvidence) || len(facts.Records) != 0 {
		t.Fatalf("unexpected success with a contradictory role flag: %+v, %v", facts, err)
	}
}
