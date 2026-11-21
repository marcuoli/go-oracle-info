package objects

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

func invalidFixture() session.Result {
	return coreResult("OWNER,OBJECT_TYPE,OBJECT_NAME,STATUS,CREATED,LAST_DDL_TIME", coreText("Case'Owner"), coreText("PACKAGE BODY"), coreText("Mixed Name"), coreText("INVALID"), coreText("2020-01-01T01:02:03"), session.Value{Null: true})
}
func TestInvalidDetailsRetainDatesAndEnforceSelectedOwnerStatus(t *testing.T) {
	source := invalidFixture()
	facts, err := DecodeInvalidDetails(source, 11, "Case'Owner")
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Created.Text != "2020-01-01T01:02:03" || !facts.Rows[0].LastDDLTime.Null {
		t.Fatalf("dates: %+v %v", facts, err)
	}
	for _, mutation := range []string{"owner", "status", "late"} {
		source = invalidFixture()
		switch mutation {
		case "owner":
			source.Rows[0][0] = coreText("Other")
		case "status":
			source.Rows[0][3] = coreText("VALID")
		case "late":
			source.Rows = append(source.Rows, nil)
		}
		facts, err = DecodeInvalidDetailsWithOptions(source, InvalidDetailsOptions{OracleMajor: 19, Owners: []string{"Case'Owner"}})
		if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
			t.Fatalf("%s: %+v %v", mutation, facts, err)
		}
	}
}
func TestInvalidDetailsOwnedReadBindsOwnerAndRejectsFailedPayload(t *testing.T) {
	ref := coreReference(t)
	failure := errors.New("private failure")
	reader := &coreFixtureReader{query: func(_ context.Context, s session.Statement) (session.Result, error) {
		if s.Reference != ref || strings.Contains(s.Text, "Case'Owner") || len(s.Args) != 1 || s.Args[0] != "Case'Owner" {
			t.Fatal("unbound owner/ref changed")
		}
		return invalidFixture(), failure
	}}
	facts, err := ReadInvalidDetails(t.Context(), reader, InvalidDetailsRequest{OracleMajor: 19, Owners: []string{"Case'Owner"}, Reference: ref})
	if facts.Rows != nil || !errors.Is(err, failure) || strings.Contains(err.Error(), "private") {
		t.Fatalf("failed prefix: %+v %v", facts, err)
	}
}
