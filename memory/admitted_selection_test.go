package memory

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// This opt-in static check consumes admitted scope evidence; it executes no SQL,
// selects no credentials and does not admit an ADB effective fallback.
func TestAdmittedMemoryStatementEvidence(t *testing.T) {
	path := os.Getenv("ORACLE_INFO_CAPABILITY_PROOF")
	if path == "" {
		t.Skip("caller-admitted environment JSON required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Frames []struct {
			Name string
			Data struct {
				Results []struct{ Items []map[string]json.RawMessage }
			}
		}
	}
	if err = json.Unmarshal(data, &proof); err != nil {
		t.Fatal(err)
	}
	version := ""
	conID := ""
	cdb := ""
	for _, f := range proof.Frames {
		if f.Name == "context" && len(f.Data.Results) == 1 && len(f.Data.Results[0].Items) == 1 {
			r := f.Data.Results[0].Items[0]
			json.Unmarshal(r["version_full"], &version)
			json.Unmarshal(r["con_id"], &conID)
			json.Unmarshal(r["cdb"], &cdb)
		}
	}
	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.Atoi(conID)
	if err != nil {
		t.Fatal(err)
	}
	scope := NonCDB
	if cdb == "YES" {
		if id == 1 {
			scope = CDBRoot
		} else if id > 2 {
			scope = ConnectedPDB
		} else {
			t.Fatal("inadmissible seed scope")
		}
	}
	for _, parameter := range []Parameter{SGA, PGA} {
		for _, inherit := range []bool{false, true} {
			if scope != ConnectedPDB && inherit {
				continue
			}
			o := Options{OracleMajor: major, Scope: scope, PDB: PDBOptions{AllowInherited: inherit}}
			if scope == ConnectedPDB {
				o.TargetContainerID = id
			}
			p, err := parameterPlan(parameter, o)
			if err != nil {
				t.Fatal(err)
			}
			name := "Scoped_" + string(parameter)
			if inherit {
				name += "_inherited"
			}
			b, _ := json.Marshal(struct {
				Package, Fact string
				Major         int
				SQL           string
				Args          []any
			}{"memory", name, major, p.sql, p.args})
			t.Log(string(b))
		}
	}
	p, err := racPlan(Options{OracleMajor: major, Scope: scope, PDB: PDBOptions{AllowInherited: scope == ConnectedPDB}, TargetContainerID: func() int {
		if scope == ConnectedPDB {
			return id
		}
		return 0
	}()})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(struct {
		Package, Fact string
		Major         int
		SQL           string
		Args          []any
	}{"memory", "RAC", major, p.sql, p.args})
	t.Log(string(b))
}
