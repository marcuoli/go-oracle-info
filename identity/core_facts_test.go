package identity

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
	"testing"
)

type coreFixtureReader struct {
	query func(context.Context, session.Statement) (session.Result, error)
	calls int
}

func (r *coreFixtureReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	r.calls++
	return r.query(ctx, s)
}
func coreReference(t *testing.T) reference.Reference {
	t.Helper()
	ref, err := reference.NewReference("unit", "core", "facts", 1)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func coreResult(columns string, values ...session.Value) session.Result {
	return session.Result{Columns: strings.Split(columns, ","), Rows: [][]session.Value{values}}
}
func coreText(value string) session.Value { return session.Value{Text: value} }

func databaseInfoFixture(pl bool) session.Result {
	names := "DBID,NAME,DB_UNIQUE_NAME,LOG_MODE,CONTROLFILE_TYPE,VERSION_TIME,OPEN_MODE,DATABASE_ROLE,SUPPLEMENTAL_LOG_DATA_MIN,SUPPLEMENTAL_LOG_DATA_PK,SUPPLEMENTAL_LOG_DATA_UI,FORCE_LOGGING,PLATFORM_ID,PLATFORM_NAME,CURRENT_SCN,FLASHBACK_ON,SUPPLEMENTAL_LOG_DATA_FK,SUPPLEMENTAL_LOG_DATA_ALL"
	values := []session.Value{coreText("4294967295"), coreText("DB"), coreText("Unique"), coreText("ARCHIVELOG"), coreText("STANDBY"), coreText("2026-11-15T01:02:03"), coreText("MOUNTED"), coreText("PHYSICAL STANDBY"), coreText("IMPLICIT"), coreText("YES"), coreText("NO"), coreText("STANDBY NOLOGGING FOR DATA AVAILABILITY"), coreText("13"), coreText("Linux x86 64-bit"), coreText("9007199254740993"), coreText("RESTORE POINT ONLY"), coreText("NO"), coreText("future")}
	if pl {
		names += ",SUPPLEMENTAL_LOG_DATA_PL"
		values = append(values, session.Value{Null: true})
	}
	return coreResult(names, values...)
}
func TestDatabaseInfoExactNumbersOptionalColumnAndUnknownFlags(t *testing.T) {
	for _, selected := range []bool{false, true} {
		options := DatabaseInfoOptions{OracleMajor: 19, IncludeSupplementalPL: selected}
		source := databaseInfoFixture(selected)
		facts, err := DecodeDatabaseInfo(source, options)
		if err != nil || len(facts.Rows) != 1 {
			t.Fatal(facts, err)
		}
		fact := facts.Rows[0]
		if fact.DBID.Text != "4294967295" || fact.CurrentSCN.Text != "9007199254740993" || fact.PlatformID.Text != "13" || fact.ControlfileType.Text != "STANDBY" || fact.VersionTime.Text != "2026-11-15T01:02:03" || fact.SupplementalAll.Text != "future" || fact.ForceLogging.Text != "STANDBY NOLOGGING FOR DATA AVAILABILITY" {
			t.Fatalf("lost properties: %+v", fact)
		}
		if selected && (fact.SupplementalPL == nil || !fact.SupplementalPL.Null) {
			t.Fatal("selected PL NULL lost")
		}
		if !selected && fact.SupplementalPL != nil {
			t.Fatal("unselected PL synthesized")
		}
		source.Rows[0][14] = session.Value{Null: true}
		facts, err = DecodeDatabaseInfo(source, options)
		if err != nil || !facts.Rows[0].CurrentSCN.Null {
			t.Fatal("mounted NULL SCN changed", err)
		}
		source.Rows = append(source.Rows, source.Rows[0])
		facts, err = DecodeDatabaseInfo(source, options)
		if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal("scalar accepted multiple rows", err)
		}
	}
	source := databaseInfoFixture(false)
	source.Rows[0][14] = coreText("1.5")
	facts, err := DecodeDatabaseInfo(source, DatabaseInfoOptions{OracleMajor: 11})
	if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("fractional SCN accepted", err)
	}
}
func TestObservedSessionContextIsNotServerOrConfiguredIdentity(t *testing.T) {
	source := coreResult("CLIENT_HOST,OS_USER,SESSION_USER", coreText("client.example"), session.Value{Null: true}, coreText("Observed User"))
	facts, err := DecodeSessionContext(source, 11)
	if err != nil || facts.Provenance.Scope != "executing_session_client_context" || facts.Rows[0].ClientHost.Text != "client.example" || !facts.Rows[0].OSUser.Null || facts.Rows[0].SessionUser.Text != "Observed User" {
		t.Fatalf("context: %+v %v", facts, err)
	}
	statement, _ := SessionContextStatement(11)
	if strings.Contains(statement, "SERVER_HOST") || strings.Contains(statement, "V$INSTANCE") || !strings.Contains(statement, "'HOST',4000") {
		t.Fatal(statement)
	}
}
func TestIdentityCoreReadRejectsLateCancellationAndRetainsError(t *testing.T) {
	ref := coreReference(t)
	failure := errors.New("private driver failure")
	for _, kind := range []string{"database", "session"} {
		ctx, cancel := context.WithCancel(t.Context())
		reader := &coreFixtureReader{query: func(_ context.Context, s session.Statement) (session.Result, error) {
			cancel()
			if s.Reference != ref {
				t.Fatal("reference changed")
			}
			return databaseInfoFixture(false), failure
		}}
		var rows int
		var err error
		if kind == "database" {
			facts, e := ReadDatabaseInfo(ctx, reader, DatabaseInfoRequest{Options: DatabaseInfoOptions{OracleMajor: 19}, Reference: ref})
			rows = len(facts.Rows)
			err = e
		} else {
			facts, e := ReadSessionContext(ctx, reader, SessionContextRequest{OracleMajor: 19, Reference: ref})
			rows = len(facts.Rows)
			err = e
		}
		cancel()
		if rows != 0 || !errors.Is(err, failure) || !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private") {
			t.Fatalf("failed read: %d %v", rows, err)
		}
	}
}
