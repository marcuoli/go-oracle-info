package services

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

func serviceFixture() session.Result {
	return coreResult("NAME,NETWORK_NAME,CREATION_DATE,CREATION_DATE_HASH,FAILOVER_METHOD,FAILOVER_TYPE,FAILOVER_RETRIES,FAILOVER_DELAY,MIN_CARDINALITY,MAX_CARDINALITY,GOAL,DTP,ENABLED,AQ_HA_NOTIFICATIONS,CLB_GOAL,EDITION", coreText("Svc"), coreText("Svc.example"), coreText("2026-11-15T01:02:03"), coreText("4294967295"), coreText("BASIC"), coreText("future"), coreText("11"), coreText("30"), session.Value{Null: true}, coreText("0"), coreText("SERVICE_TIME"), coreText("N"), coreText("UNKNOWN"), coreText("YES"), coreText("LONG"), session.Value{Null: true})
}
func TestServiceDecodeKeepsSixteenColumnsAndRejectsLateNumericRows(t *testing.T) {
	source := serviceFixture()
	facts, err := Decode(source, 11, true)
	if err != nil || len(facts.Rows) != 1 {
		t.Fatal(facts, err)
	}
	service := facts.Rows[0]
	if service.Name.Text != "Svc" || service.NetworkName.Text != "Svc.example" || service.CreationDate.Text != "2026-11-15T01:02:03" || service.CreationDateHash.Text != "4294967295" || service.FailoverRetries.Text != "11" || service.FailoverDelay.Text != "30" || service.FailoverType.Text != "future" || !service.MinCardinality.Null || (service.Edition == nil || !service.Edition.Null) || service.Enabled.Text != "UNKNOWN" {
		t.Fatalf("lost service fields: %+v", service)
	}
	for _, bad := range []string{"1.1", "1e3", "9223372036854775808"} {
		late := append([]session.Value(nil), source.Rows[0]...)
		late[3] = coreText(bad)
		source.Rows = append(source.Rows[:1], late)
		facts, err = Decode(source, 26)
		if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
			t.Fatalf("%q retained prefix: %+v %v", bad, facts, err)
		}
	}
}
func TestServiceReadRequiresCompleteUncancelledReader(t *testing.T) {
	ref := coreReference(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := &coreFixtureReader{query: func(_ context.Context, statement session.Statement) (session.Result, error) {
		want, _ := Statement(19)
		if statement.Text != want || statement.Reference != ref {
			t.Fatal("SQL/ref changed")
		}
		cancel()
		return serviceFixture(), nil
	}}
	facts, err := Read(ctx, reader, Request{OracleMajor: 19, Reference: ref})
	if facts.Rows != nil || !errors.Is(err, context.Canceled) || reader.calls != 1 {
		t.Fatalf("cancelled: %+v %v", facts, err)
	}
}

func TestServiceLegacyEditionIsUnselectedWithoutCapability(t *testing.T) {
	source := serviceFixture()
	source.Columns = source.Columns[:15]
	source.Rows[0] = source.Rows[0][:15]
	facts, err := Decode(source, 11)
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Edition != nil {
		t.Fatalf("legacy optional column: %+v %v", facts, err)
	}
	sql, err := Statement(11)
	if err != nil || strings.Contains(sql, "EDITION AS EDITION") {
		t.Fatal("major11 invented patch capability", sql, err)
	}
	sql, err = Statement(11, true)
	if err != nil || !strings.Contains(sql, "EDITION AS EDITION") {
		t.Fatal("explicit column omitted", sql, err)
	}
	if sql, err = Statement(11, true, false); sql != "" || !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("ambiguous admission accepted", sql, err)
	}
}
