package grants

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestRoleCatalogReleaseBoundary(t *testing.T) {
	for _, version := range []string{"9.2", "10.1", "10.2", "11.1", "11.2", "12.1", "12.1.0.1.0", "12.2", "12.2.0.0.0", "26.0.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			stmt, args, err := RoleCatalogStatement(version)
			if !errors.Is(err, ErrRoleCatalogUnavailable) || stmt != "" || args != nil {
				t.Fatalf("unsupported release selected SQL: %q, %v, %v", stmt, args, err)
			}
			reader := fixtureReader(func(context.Context, session.Statement) (session.Result, error) {
				t.Fatal("unsupported release reached the reader")
				return session.Result{}, nil
			})
			facts, err := ReadRoleCatalog(t.Context(), reader, RoleCatalogRequest{OracleRelease: version})
			if !errors.Is(err, ErrRoleCatalogUnavailable) || facts.Records != nil {
				t.Fatalf("unsupported release became a successful result: %+v, %v", facts, err)
			}
		})
	}
	for _, version := range []string{"12.1.0.2", "12.2.0.1.0", "18.0", "19.32.0.0.0", "21.0", "23.9.0.0.0", "23.26.1.0.0"} {
		t.Run(version, func(t *testing.T) {
			const expected = "SELECT r.ROLE AS ROLE, r.ORACLE_MAINTAINED AS ORACLE_MAINTAINED " +
				"FROM DBA_ROLES r ORDER BY NLSSORT(r.ROLE, 'NLS_SORT=BINARY')"
			stmt, args, err := RoleCatalogStatement(version)
			if err != nil || stmt != expected || len(args) != 0 {
				t.Fatalf("unexpected standalone DBA_ROLES selection: %q, %v, %v", stmt, args, err)
			}
		})
	}
	for _, version := range []string{"", "12", "26ai", "19.0.0.0.0.0", "19.03.0", "12.x.0.2.0"} {
		if stmt, _, err := RoleCatalogStatement(version); !errors.Is(err, ErrInvalidRequest) || stmt != "" {
			t.Errorf("malformed release %q produced %q, %v", version, stmt, err)
		}
	}
}

func TestRoleCatalogReadRetainsStandaloneEvidence(t *testing.T) {
	const version = "23.26.1.0.0"
	ref, err := reference.NewReference("unit", "facts", "role-catalog", 1)
	if err != nil {
		t.Fatal(err)
	}
	input := classifiedFixture(roleCatalog,
		map[string]string{"ROLE": "Mixed Case role", "ORACLE_MAINTAINED": "N"},
		map[string]string{"ROLE": "DBA", "ORACLE_MAINTAINED": "Y"},
		map[string]string{"ROLE": "FLAG_UNKNOWN"},
	)
	calls := 0
	reader := fixtureReader(func(_ context.Context, statement session.Statement) (session.Result, error) {
		calls++
		if statement.Reference != ref || statement.Text != roleCatalog.sql || len(statement.Args) != 0 {
			t.Fatalf("unexpected query: %+v", statement)
		}
		return input, nil
	})
	request := RoleCatalogRequest{OracleRelease: version, Reference: ref}
	facts, err := ReadRoleCatalog(t.Context(), reader, request)
	want := RoleCatalogResult{
		Provenance: RoleCatalogProvenance{Source: "DBA_ROLES", OracleMajor: 23, OracleRelease: version, Scope: "connected_container_dba_dictionary"},
		Records: []RoleCatalogEntry{
			{Role: session.Value{Text: "Mixed Case role"}, OracleMaintained: session.Value{Text: "N"}},
			{Role: session.Value{Text: "DBA"}, OracleMaintained: session.Value{Text: "Y"}},
			{Role: session.Value{Text: "FLAG_UNKNOWN"}, OracleMaintained: session.Value{Null: true}},
		},
	}
	if err != nil || calls != 1 || !reflect.DeepEqual(facts, want) {
		t.Fatalf("read changed dictionary values: %+v, %v", facts, err)
	}

	request.Limits.MaxRows = 1
	facts, err = ReadRoleCatalog(t.Context(), reader, request)
	if !errors.Is(err, rowset.ErrCapacity) || facts.Records != nil {
		t.Fatalf("over-budget read published a result: %+v, %v", facts, err)
	}

	request.Limits = rowset.Limits{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	facts, err = ReadRoleCatalog(ctx, fixtureReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return input, nil
	}), request)
	if !errors.Is(err, context.Canceled) || facts.Records != nil {
		t.Fatalf("cancellation published a result: %+v, %v", facts, err)
	}

	denied := errors.New("dictionary access rejected")
	facts, err = ReadRoleCatalog(t.Context(), fixtureReader(func(context.Context, session.Statement) (session.Result, error) {
		return input, denied
	}), request)
	if !errors.Is(err, denied) || facts.Records != nil || facts.Provenance.Source != "" {
		t.Fatalf("failed access published a result: %+v, %v", facts, err)
	}
}

func TestRoleCatalogDecodeRejectsInvalidLateEvidence(t *testing.T) {
	const version = "19.32.0.0.0"
	good := map[string]string{"ROLE": "APP_ROLE", "ORACLE_MAINTAINED": "N"}
	for _, bad := range []map[string]string{
		{"ORACLE_MAINTAINED": "N"},
		{"ROLE": "", "ORACLE_MAINTAINED": "N"},
		{"ROLE": "OTHER", "ORACLE_MAINTAINED": "?"},
	} {
		input := classifiedFixture(roleCatalog, good, bad)
		result, err := DecodeRoleCatalog(input, version)
		if !errors.Is(err, ErrInvalidEvidence) || result.Records != nil || result.Provenance.Source != "" {
			t.Fatalf("malformed late row published a prefix: %+v, %v", result, err)
		}
	}
	input := classifiedFixture(roleCatalog)
	empty, err := DecodeRoleCatalog(input, version)
	if err != nil || empty.Records == nil || len(empty.Records) != 0 || empty.Provenance.Source != "DBA_ROLES" {
		t.Fatalf("empty dictionary result lost its provenance: %+v, %v", empty, err)
	}
	input.Columns[1] = "OTHER"
	if result, err := DecodeRoleCatalog(input, version); !errors.Is(err, ErrInvalidEvidence) || result.Records != nil {
		t.Fatalf("missing maintenance column yielded a result: %+v, %v", result, err)
	}
}

func TestRoleCatalogNativeRowsAndJSON(t *testing.T) {
	const version = "23.26.1.0.0"
	step := testsql.Step{
		SQL:     roleCatalog.sql,
		Columns: []string{"ROLE", "ORACLE_MAINTAINED"},
		Values:  [][]driver.Value{{"Mixed Case role", nil}},
	}
	db, state := testsql.New(t, step)
	rows, err := RoleCatalogRows(t.Context(), db, version)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var name string
	var flag *string
	if !rows.Next() || rows.Scan(&name, &flag) != nil || name != "Mixed Case role" || flag != nil ||
		rows.Next() || rows.Err() != nil || rows.Close() != nil || state.Closed() != 1 {
		t.Fatalf("native cursor changed role or NULL: %q, %v, close count %d", name, flag, state.Closed())
	}
	db, state = testsql.New(t, step)
	payload, err := RoleCatalogJSON(t.Context(), db, version, rowset.Limits{})
	var envelope struct {
		Provenance RoleCatalogProvenance
		Data       struct {
			Columns []rowset.Column
			Rows    [][]*string
		}
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || state.Closed() != 1 ||
		envelope.Provenance.OracleRelease != version || !reflect.DeepEqual(envelope.Data.Columns, roleCatalog.schema) ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 2 ||
		envelope.Data.Rows[0][0] == nil || *envelope.Data.Rows[0][0] != "Mixed Case role" || envelope.Data.Rows[0][1] != nil {
		t.Fatalf("unexpected catalog JSON: %s, %v", payload, err)
	}
	step.Values = append(step.Values, []driver.Value{"Another role", "?"})
	db, state = testsql.New(t, step)
	payload, err = RoleCatalogJSON(t.Context(), db, version, rowset.Limits{})
	if !errors.Is(err, ErrInvalidEvidence) || payload != nil || state.Closed() != 1 {
		t.Fatalf("malformed native row published JSON: %s, %v", payload, err)
	}
}
