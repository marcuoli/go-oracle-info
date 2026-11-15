package asm

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"strings"
	"testing"
)

func TestClientRolesInstanceScopeAndRawVersions(t *testing.T) {
	if _, _, err := ClientsStatement(19, Filter{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing role admitted: %v", err)
	}
	filter := Filter{Instances: []int64{1, 2}, SourceRole: DatabaseInstance}
	plan, _ := clientsPlan(19, filter)
	source := sourceResult(plan)
	for i, name := range source.Columns {
		switch name {
		case "SOFTWARE_VERSION":
			source.Rows[0][i] = session.Value{Text: "19.10.0.0.0"}
		case "COMPATIBLE_VERSION":
			source.Rows[0][i] = session.Value{Null: true}
		}
	}
	facts, err := DecodeClients(source, 19, filter)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Rows[0].SoftwareVersion.Text != "19.10.0.0.0" || !facts.Rows[0].CompatibleVersion.Null || facts.Provenance.SourceRole != "database_instance" || strings.Contains(plan.sql, "MIN(") {
		t.Fatalf("altered connection/version facts: %+v", facts)
	}
	late := append([]session.Value(nil), source.Rows[0]...)
	late[0] = session.Value{Text: "3"}
	source.Rows = append(source.Rows, late)
	bad, err := DecodeClients(source, 19, filter)
	if !errors.Is(err, ErrInvalidEvidence) || bad.Rows != nil {
		t.Fatalf("accepted outside selected instance: %+v %v", bad, err)
	}
	legacy, _ := clientsPlan(11, Filter{SourceRole: ASMInstance})
	old, err := DecodeClients(sourceResult(legacy), 11, Filter{SourceRole: ASMInstance})
	if err != nil || old.Rows[0].ContainerID != nil || old.Provenance.SourceRole != "asm_instance" {
		t.Fatalf("legacy source role/absence lost: %+v %v", old, err)
	}
}
func TestClientReaderNoQueryOrCancellationPrefix(t *testing.T) {
	plan := testPlan(t)
	source := sourceResult(plan)
	sentinel := errors.New("private")
	request := Request{OracleMajor: 19, Filter: Filter{SourceRole: DatabaseInstance}, Reference: sourceRef(t)}
	facts, err := ReadClients(context.Background(), &fixtureReader{result: source, err: sentinel}, request)
	if !errors.Is(err, sentinel) || facts.Rows != nil {
		t.Fatalf("query prefix: %+v %v", facts, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	facts, err = ReadClients(ctx, &fixtureReader{result: source, after: cancel}, request)
	if !errors.Is(err, context.Canceled) || facts.Rows != nil {
		t.Fatalf("cancel prefix: %+v %v", facts, err)
	}
}
