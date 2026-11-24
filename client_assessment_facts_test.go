package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/users"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const assessmentVersion = "23.26.1.0.0"

type assessmentReader func(context.Context, session.Statement) (session.Result, error)

func (f assessmentReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}

func TestAssessmentFactReleaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		version                      string
		startup, password, encrypted bool
	}{
		{"9.2.0.8.0", true, false, false},
		{"10.1.0.5.0", true, false, false},
		{"10.2.0.1.0", true, false, true},
		{"10.2.0.5.0", true, false, true},
		{"11.1.0.6.0", true, true, true},
		{"11.2.0.4.0", true, true, true},
		{"12.1.0.2.0", true, true, true},
		{"12.2.0.1.0", true, true, true},
		{"18.0.0.0.0", true, true, true},
		{"19.32.0.0.0", true, true, true},
		{"21.3.0.0.0", true, true, true},
		{"23.2.0.0.0", true, true, true},
		{"23.9.0.0.0", true, true, true},
		{"23.26.1.0.0", true, true, true},
		{"23.26.4.0.0", true, true, true},
		{"26.0.0.0.0", false, false, false},
		{"10", false, false, false},
		{"23ai", false, false, false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			member, memberErr := instances.MemberStartupStatement(instances.MemberStartupOptions{OracleVersion: tc.version})
			password, passwordErr := users.PasswordVersionCountsStatement(users.PasswordVersionCountsOptions{OracleVersion: tc.version})
			encrypted, encryptedErr := objects.EncryptedColumnsStatement(objects.EncryptedColumnsOptions{OracleVersion: tc.version})
			for _, got := range []struct {
				sql              string
				err, unsupported error
				supported        bool
			}{
				{member, memberErr, instances.ErrUnsupportedVersion, tc.startup},
				{password, passwordErr, users.ErrUnsupportedVersion, tc.password},
				{encrypted, encryptedErr, objects.ErrUnsupportedVersion, tc.encrypted},
			} {
				if got.supported && (got.err != nil || got.sql == "") ||
					!got.supported && (!errors.Is(got.err, got.unsupported) || got.sql != "") {
					t.Fatalf("wrong release selection: %q, %v", got.sql, got.err)
				}
			}
			if tc.startup && (!strings.HasPrefix(member, "SELECT i.INST_ID AS INST_ID, CASE WHEN i.STARTUP_TIME IS NULL THEN NULL") ||
				!strings.HasSuffix(member, " AS STARTUP_TIME, i.STATUS AS STATUS FROM SYS.GV_$INSTANCE i ORDER BY i.INST_ID")) {
				t.Fatalf("unexpected member projection: %s", member)
			}
			if tc.password && (!strings.HasPrefix(password, "SELECT u.PASSWORD_VERSIONS AS PASSWORD_VERSIONS, TO_CHAR(COUNT(*)") ||
				!strings.Contains(password, " AS ACCOUNT_COUNT FROM SYS.DBA_USERS u GROUP BY u.PASSWORD_VERSIONS, NLSSORT(") || strings.Contains(password, "USERNAME")) {
				t.Fatalf("unexpected aggregate projection: %s", password)
			}
			const encryptedSQL = "SELECT v.OWNER AS OWNER, v.TABLE_NAME AS TABLE_NAME, v.COLUMN_NAME AS COLUMN_NAME FROM SYS.DBA_ENCRYPTED_COLUMNS v ORDER BY v.OWNER, v.TABLE_NAME, v.COLUMN_NAME"
			if tc.encrypted && encrypted != encryptedSQL {
				t.Fatalf("unexpected encrypted metadata projection: %s", encrypted)
			}
		})
	}
}

func TestAssessmentFactDecodersPreserveEvidence(t *testing.T) {
	null := session.Value{Null: true}
	text := func(s string) session.Value { return session.Value{Text: s} }
	member := session.Result{
		Columns: []string{"STATUS", "INST_ID", "STARTUP_TIME"},
		Rows:    [][]session.Value{{text("OPEN"), text("7"), text("2024-02-29T03:01:02")}, {null, text("7"), null}},
	}
	options := instances.MemberStartupOptions{OracleVersion: assessmentVersion}
	m, err := instances.DecodeMemberStartup(member, options)
	if err != nil || m.State != instances.Available || m.Provenance.Complete ||
		!reflect.DeepEqual(m.Rows, []instances.MemberStartupRow{
			{InstanceID: text("7"), StartupTime: text("2024-02-29T03:01:02"), Status: text("OPEN")},
			{InstanceID: text("7"), StartupTime: null, Status: null},
		}) {
		t.Fatalf("startup = %#v, %v", m, err)
	}
	member.Rows[1][2] = text("2025-02-29T00:01:02")
	if bad, err := instances.DecodeMemberStartup(member, options); !errors.Is(err, instances.ErrInvalidEvidence) || bad.Rows != nil {
		t.Fatalf("late invalid date produced partial evidence: %#v, %v", bad, err)
	}

	pwopts := users.PasswordVersionCountsOptions{OracleVersion: assessmentVersion}
	wanted := []struct {
		v     session.Value
		class users.PasswordVersionClass
	}{
		{null, users.PasswordVersionsUnreported},
		{text("10G HTTP "), users.PasswordVersions10GOnly},
		{text("10G 11G 12C"), users.PasswordVersionsCaseSensitive},
		{text("HTTP"), users.PasswordVersionsHTTPOnly},
		{text("10G LATER"), users.PasswordVersionsUnknown},
		{text(""), users.PasswordVersionsUnknown},
	}
	const count = "184467440737095516160"
	pw := session.Result{Columns: []string{"PASSWORD_VERSIONS", "ACCOUNT_COUNT"}}
	for _, w := range wanted {
		pw.Rows = append(pw.Rows, []session.Value{w.v, text(count)})
	}
	p, err := users.DecodePasswordVersionCounts(pw, pwopts)
	if err != nil || len(p.Rows) != len(wanted) || p.Provenance.Scope != "connected_dictionary" {
		t.Fatalf("password groups = %#v, %v", p, err)
	}
	for i, row := range p.Rows {
		if row.Versions != wanted[i].v || row.AccountCount.Text != count || row.Class() != wanted[i].class {
			t.Fatalf("group %d = %#v, class %q", i, row, row.Class())
		}
	}
	pw.Rows = append(pw.Rows, []session.Value{null, text("1")})
	if bad, err := users.DecodePasswordVersionCounts(pw, pwopts); !errors.Is(err, users.ErrInvalidEvidence) || bad.Rows != nil {
		t.Fatalf("duplicate NULL groups produced evidence: %#v, %v", bad, err)
	}

	eopts := objects.EncryptedColumnsOptions{OracleVersion: "10.2.0.1.0"}
	enc := session.Result{
		Columns: []string{"OWNER", "TABLE_NAME", "COLUMN_NAME"},
		Rows:    [][]session.Value{{text("Quoted Owner"), text("T"), text("Secret Label")}, {text("Owner"), text("X"), null}},
	}
	e, err := objects.DecodeEncryptedColumns(enc, eopts)
	if err != nil || e.Provenance.Complete || !reflect.DeepEqual(e.Rows, []objects.EncryptedColumn{
		{Owner: text("Quoted Owner"), TableName: text("T"), ColumnName: text("Secret Label")},
		{Owner: text("Owner"), TableName: text("X"), ColumnName: null},
	}) {
		t.Fatalf("encrypted metadata = %#v, %v", e, err)
	}
}

func TestAssessmentFactReadersRejectCapacityAndQueryFailure(t *testing.T) {
	ref, err := reference.NewReference("oracleinfo", "fixture", "query", 197)
	if err != nil {
		t.Fatal(err)
	}
	privilege := errors.New("ORA-01031: private diagnostic")
	for _, tc := range []struct {
		name         string
		columns      []string
		read         func(context.Context, assessmentReader, rowset.Limits) (int, error)
		inaccessible error
	}{
		{"member", []string{"INST_ID", "STARTUP_TIME", "STATUS"}, func(ctx context.Context, r assessmentReader, lim rowset.Limits) (int, error) {
			got, err := instances.ReadMemberStartup(ctx, r, instances.MemberStartupRequest{Options: instances.MemberStartupOptions{OracleVersion: assessmentVersion}, Reference: ref, Limits: lim})
			return len(got.Rows), err
		}, instances.ErrSourceInaccessible},
		{"password", []string{"PASSWORD_VERSIONS", "ACCOUNT_COUNT"}, func(ctx context.Context, r assessmentReader, lim rowset.Limits) (int, error) {
			got, err := users.ReadPasswordVersionCounts(ctx, r, users.PasswordVersionCountsRequest{Options: users.PasswordVersionCountsOptions{OracleVersion: assessmentVersion}, Reference: ref, Limits: lim})
			return len(got.Rows), err
		}, users.ErrSourceInaccessible},
		{"encrypted", []string{"OWNER", "TABLE_NAME", "COLUMN_NAME"}, func(ctx context.Context, r assessmentReader, lim rowset.Limits) (int, error) {
			got, err := objects.ReadEncryptedColumns(ctx, r, objects.EncryptedColumnsRequest{Options: objects.EncryptedColumnsOptions{OracleVersion: assessmentVersion}, Reference: ref, Limits: lim})
			return len(got.Rows), err
		}, objects.ErrSourceInaccessible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := session.Result{Columns: tc.columns}
			if n, err := tc.read(t.Context(), func(context.Context, session.Statement) (session.Result, error) { return result, nil }, rowset.Limits{}); err != nil || n != 0 {
				t.Fatalf("empty = %d, %v", n, err)
			}
			result.Rows = make([][]session.Value, 2)
			if n, err := tc.read(t.Context(), func(context.Context, session.Statement) (session.Result, error) { return result, nil }, rowset.Limits{MaxRows: 1}); !errors.Is(err, rowset.ErrCapacity) || n != 0 {
				t.Fatalf("capacity = %d, %v", n, err)
			}
			if n, err := tc.read(t.Context(), func(context.Context, session.Statement) (session.Result, error) { return result, privilege }, rowset.Limits{}); !errors.Is(err, privilege) || !errors.Is(err, tc.inaccessible) || n != 0 || strings.Contains(err.Error(), "private diagnostic") {
				t.Fatalf("privilege = %d, %v", n, err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if n, err := tc.read(ctx, func(context.Context, session.Statement) (session.Result, error) { cancel(); return result, nil }, rowset.Limits{}); !errors.Is(err, context.Canceled) || n != 0 {
				t.Fatalf("cancellation = %d, %v", n, err)
			}
		})
	}
}

func TestAssessmentPasswordJSONRetainsExactCountAndCursorFailure(t *testing.T) {
	options := users.PasswordVersionCountsOptions{OracleVersion: assessmentVersion}
	statement, err := users.PasswordVersionCountsStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	source := testsql.Step{SQL: statement, Columns: []string{"PASSWORD_VERSIONS", "ACCOUNT_COUNT"}, Values: [][]driver.Value{{nil, "184467440737095516160"}, {"10G HTTP ", "2"}}}
	broken := source
	cause := errors.New("terminal cursor failure")
	broken.NextErr = cause
	db, seen := testsql.New(t, source, broken)
	payload, err := users.PasswordVersionCountsJSON(t.Context(), db, options, info.JSONLimits{MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			Columns []rowset.Column
			Rows    [][]any
		}
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Data.Columns, []rowset.Column{{Name: "PASSWORD_VERSIONS", Kind: rowset.Text}, {Name: "ACCOUNT_COUNT", Kind: rowset.Text}}) ||
		!reflect.DeepEqual(result.Data.Rows, [][]any{{nil, "184467440737095516160"}, {"10G HTTP ", "2"}}) {
		t.Fatalf("wrong JSON projection: %s", payload)
	}
	if failed, err := users.PasswordVersionCountsJSON(t.Context(), db, options, info.JSONLimits{}); !errors.Is(err, cause) || failed != nil || seen.Closed() != 2 {
		t.Fatalf("failed cursor leaked evidence: %s, %v, %d closes", failed, err, seen.Closed())
	}
}
