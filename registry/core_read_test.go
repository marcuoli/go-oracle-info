package registry

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
	"testing"
)

type coreReader struct {
	query func(context.Context, session.Statement) (session.Result, error)
}

func (r *coreReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return r.query(ctx, s)
}
func TestComponentOwnedReadPreservesPublishedFactsAndRejectsFailedPayload(t *testing.T) {
	ref, err := reference.NewReference("unit", "registry", "components", 1)
	if err != nil {
		t.Fatal(err)
	}
	source := session.Result{Columns: []string{"COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"}, Rows: [][]session.Value{{{Text: "CATALOG"}, {Text: "Dictionary"}, {Text: "19.0.0.0.0"}, {Null: true}, {Text: "future status"}, {Text: " raw local label "}}}}
	for _, mode := range []string{"success", "error", "late", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("private failure")
			reader := &coreReader{query: func(_ context.Context, s session.Statement) (session.Result, error) {
				want, _ := Statement(19)
				if s.Text != want || s.Reference != ref || len(s.Args) != 0 {
					t.Fatal("changed reference/statement")
				}
				result := source
				switch mode {
				case "error":
					return result, failure
				case "late":
					result.Rows = append(append([][]session.Value(nil), source.Rows...), nil)
				case "cancel":
					cancel()
				}
				return result, nil
			}}
			facts, err := Read(ctx, reader, Request{OracleMajor: 19, Reference: ref})
			if mode == "success" {
				if err != nil || len(facts.Rows) != 1 || facts.Rows[0].VersionFull == nil || facts.Rows[0].VersionFull.Valid || facts.Rows[0].Status.String != "future status" || facts.Rows[0].Modified.String != " raw local label " || facts.Provenance.Source != "DBA_REGISTRY" {
					t.Fatalf("facts: %+v %v", facts, err)
				}
				return
			}
			if err == nil || facts.Rows != nil {
				t.Fatalf("failed prefix retained: %+v %v", facts, err)
			}
			if mode == "error" && (!errors.Is(err, failure) || strings.Contains(err.Error(), "private")) {
				t.Fatal("failure cause lost/leaked", err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
