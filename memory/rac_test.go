package memory_test

import (
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/session"
	"strings"
	"testing"
)

func racRow(name, value, instance, source, target string) []session.Value {
	return []session.Value{{Text: name}, {Text: value}, {Text: instance}, {Text: source}, {Text: target}}
}
func racResult(rows ...[]session.Value) session.Result {
	return session.Result{Columns: []string{"PARAMETER_NAME", "PARAMETER_VALUE", "INSTANCE_ID", "SOURCE_CONTAINER_ID", "TARGET_CONTAINER_ID"}, Rows: rows}
}
func TestRACTotalsSumBytesBeforeRoundingAndRetainMembers(t *testing.T) {
	input := racResult(
		racRow("sga_target", "300000", "1", "3", "3"), racRow("pga_aggregate_target", "0", "1", "3", "3"),
		racRow("sga_target", "300000", "2", "3", "3"), racRow("pga_aggregate_target", "1073741824", "2", "3", "3"),
	)
	report, err := memory.DecodeRAC(input, memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19, PDB: memory.PDBOptions{AllowInherited: true}})
	if err != nil || report.SGA.State != memory.Available || report.SGA.ValueGiB != "0.001" || report.PGA.ValueGiB != "1" || len(report.Members) != 2 || len(report.SGA.Values) != 2 {
		t.Fatalf("%+v err=%v", report, err)
	}
	input.Rows[0][1].Text = "mutated"
	if report.SGA.Values[0].ValueBytes != "300000" {
		t.Fatal("member evidence borrowed input")
	}
}
func TestRACInvalidMetricRetainsIndependentTotalAndSelectedEvidence(t *testing.T) {
	input := racResult(
		racRow("sga_target", "0", "1", "3", "3"), racRow("sga_max_size", "8589934592", "1", "0", "3"), racRow("pga_aggregate_target", "1073741824", "1", "3", "3"),
		racRow("sga_target", "0", "2", "3", "3"), racRow("sga_target", "0", "2", "3", "3"), racRow("sga_max_size", "8589934592", "2", "0", "3"), racRow("pga_aggregate_target", "1073741824", "2", "3", "3"),
	)
	report, err := memory.DecodeRAC(input, memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19, PDB: memory.PDBOptions{AllowInherited: true}})
	if err == nil || report.SGA.State != memory.Unavailable || report.SGA.ValueGiB != "" || report.PGA.ValueGiB != "2" || len(report.SGA.Values) != 1 {
		t.Fatalf("ambiguous child promoted or independent evidence lost: %+v %v", report, err)
	}
	if report.SGA.Values[0].Origin != memory.InheritedInstance {
		t.Fatal("inheritance provenance lost")
	}
}
func TestRACMissingMemberCannotBecomePartialSum(t *testing.T) {
	input := racResult(racRow("sga_max_size", "1073741824", "1", "0", "1"), racRow("pga_aggregate_target", "0", "1", "0", "1"), racRow("sga_max_size", "1073741824", "2", "0", "1"))
	report, err := memory.DecodeRAC(input, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19})
	if err == nil || report.SGA.ValueGiB != "2" || report.PGA.State != memory.Unavailable || report.PGA.ValueGiB != "" {
		t.Fatalf("%+v %v", report, err)
	}
}
func TestRACSQLPreservesMembershipAndVersionScope(t *testing.T) {
	for _, o := range []memory.Options{{Scope: memory.NonCDB, OracleMajor: 9}, {Scope: memory.CDBRoot, OracleMajor: 19}, {Scope: memory.ConnectedPDB, OracleMajor: 19}, {Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4}} {
		sql, err := memory.RACStatement(o)
		if err != nil || !strings.Contains(sql, "LEFT JOIN") || !strings.Contains(sql, "PARAMETER_VALUE") {
			t.Fatalf("%+v %v %s", o, err, sql)
		}
		if o.Scope == memory.ConnectedPDB || o.Scope == memory.DiscoveredPDB {
			if !strings.Contains(sql, "GV$PDBS") || !strings.Contains(sql, "OPEN_MODE IN ('READ WRITE','READ ONLY')") {
				t.Fatal("PDB open-member scope changed")
			}
		}
		if o.OracleMajor < 12 && strings.Contains(sql, "P.CON_ID") {
			t.Fatal("legacy catalog used container column")
		}
	}
}
