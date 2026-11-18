package goldengate

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"strings"
	"testing"
)

func administratorsFixture() session.Result {
	return session.Result{Columns: []string{"USERNAME", "PRIVILEGE_TYPE", "GRANT_SELECT_PRIVILEGES"}, Rows: [][]session.Value{{{Text: "GGADMIN"}, {Text: "*"}, {Text: "YES"}}, {{Text: "Quoted mixed Case"}, {Text: "CAPTURE"}, {Null: true}}, {{Text: "GGADMIN"}, {Text: "APPLY"}, {Text: "NO"}}}}
}
func TestAdministratorSourceSelection(t *testing.T) {
	for _, version := range []string{"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0", "12.1.0.2.0", "26.1.0.0.0", "19", "19.0.bad.0.0"} {
		t.Run(version, func(t *testing.T) {
			sql, err := AdministratorsStatement(version)
			if !errors.Is(err, ErrUnsupportedVersion) || sql != "" {
				t.Fatalf("unreviewed source admitted: %q %v", sql, err)
			}
		})
	}
	for _, version := range []string{"12.2.0.1.0", "18.0.0.0.0", "19.32.0.0.0", "21.3.0.0.0", "23.26.1.0.0"} {
		t.Run(version, func(t *testing.T) {
			sql, err := AdministratorsStatement(version)
			want := "SELECT v.USERNAME AS USERNAME, v.PRIVILEGE_TYPE AS PRIVILEGE_TYPE, v.GRANT_SELECT_PRIVILEGES AS GRANT_SELECT_PRIVILEGES FROM DBA_GOLDENGATE_PRIVILEGES v ORDER BY v.USERNAME, v.PRIVILEGE_TYPE, v.GRANT_SELECT_PRIVILEGES"
			if err != nil || sql != want || strings.Contains(sql, "INST_ID") || strings.Contains(sql, "GGSCHEMA") {
				t.Fatalf("wrong administrator source: %s %v", sql, err)
			}
		})
	}
}
func TestPropertiesKeepAdministratorAndConfiguredSchemaSeparate(t *testing.T) {
	reader := &readerFixture{result: administratorsFixture()}
	request := Request{OracleVersion: "19.32.0.0.0", Reference: ref(t)}
	facts, err := ReadProperties(t.Context(), reader, request)
	if err != nil || !facts.Administrators.Provenance.Complete || len(facts.Administrators.Rows) != 3 || reader.calls != 1 {
		t.Fatalf("lost complete source %+v %v", facts, err)
	}
	if facts.ConfiguredSchema.State != SchemaUnavailable || facts.ConfiguredSchema.Name != nil || facts.ConfiguredSchema.Reason != "not_exposed_by_database_source" {
		t.Fatalf("invented GGSCHEMA from administrator: %+v", facts.ConfiguredSchema)
	}
	if facts.Administrators.Rows[1].Username.Text != "Quoted mixed Case" || !facts.Administrators.Rows[1].GrantSelectPrivileges.Null || facts.Administrators.Rows[2].Username.Text != "GGADMIN" {
		t.Fatalf("changed identifiers/null/order: %+v", facts.Administrators.Rows)
	}
	reader.result.Rows = nil
	facts, err = ReadProperties(t.Context(), reader, request)
	if err != nil || !facts.Administrators.Provenance.Complete || len(facts.Administrators.Rows) != 0 || facts.ConfiguredSchema.State != SchemaUnavailable {
		t.Fatalf("empty means absence of metadata rows only: %+v %v", facts, err)
	}
}
func TestAdministratorReadFailureNeverPublishesPrefix(t *testing.T) {
	source := administratorsFixture()
	bad := append([]session.Value(nil), source.Rows[0]...)
	bad[0] = session.Value{Null: true}
	source.Rows = append(source.Rows, bad)
	facts, err := DecodeAdministrators(source, "19.32.0.0.0")
	if !errors.Is(err, ErrInvalidEvidence) || len(facts.Rows) != 0 {
		t.Fatalf("late malformed row admitted: %+v %v", facts, err)
	}
	reader := &readerFixture{result: administratorsFixture()}
	ctx, cancel := context.WithCancel(t.Context())
	reader.after = cancel
	request := Request{OracleVersion: "19.32.0.0.0", Reference: ref(t)}
	props, err := ReadProperties(ctx, reader, request)
	if !errors.Is(err, context.Canceled) || props.Administrators.Provenance.Complete || len(props.Administrators.Rows) != 0 {
		t.Fatalf("canceled read published rows: %+v %v", props, err)
	}
	cause := errors.New("terminal query failure")
	reader.after = nil
	reader.err = cause
	facts, err = ReadAdministrators(t.Context(), reader, request)
	if !errors.Is(err, cause) || facts.Provenance.Complete || len(facts.Rows) != 0 {
		t.Fatalf("query failure published rows %+v %v", facts, err)
	}
}
