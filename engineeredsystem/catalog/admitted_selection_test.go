package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// This opt-in static selection check consumes an already admitted JSON metadata
// observation. It opens no connection, reads no profiles and executes no SQL.
func TestAdmittedCapabilityStatementEvidence(t *testing.T) {
	path := os.Getenv("ORACLE_INFO_CAPABILITY_PROOF")
	if path == "" {
		t.Skip("caller-admitted metadata JSON required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("capability proof unreadable")
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
	schema := Schema{}
	major := 0
	for _, f := range proof.Frames {
		if len(f.Data.Results) != 1 {
			t.Fatal("complete single result required")
		}
		for _, r := range f.Data.Results[0].Items {
			if f.Name == "context" {
				var version string
				if err = json.Unmarshal(r["version_full"], &version); err != nil {
					t.Fatal(err)
				}
				for _, c := range version {
					if c == '.' {
						break
					}
					if c < '0' || c > '9' {
						t.Fatal("numeric version required")
					}
					major = major*10 + int(c-'0')
				}
			}
			if f.Name == "capabilities" {
				var relation, column, kind string
				if json.Unmarshal(r["source_name"], &relation) != nil || json.Unmarshal(r["column_name"], &column) != nil || json.Unmarshal(r["data_type"], &kind) != nil {
					t.Fatal("malformed admitted metadata")
				}
				if schema[relation] == nil {
					schema[relation] = map[string]string{}
				}
				schema[relation][column] = kind
			}
		}
	}
	if err = datasetMajor(major); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	proofSHA := hex.EncodeToString(sum[:])
	cases := []struct {
		name      string
		selectSQL func() (string, []any, error)
	}{
		{"Cells", func() (string, []any, error) { return CellsStatement(CellsOptions{OracleMajor: major, Schema: schema}) }},
		{"Configuration", func() (string, []any, error) {
			return ConfigurationStatement(ConfigurationOptions{OracleMajor: major, Schema: schema})
		}},
		{"ConfigurationInfo", func() (string, []any, error) {
			return ConfigurationStatement(ConfigurationOptions{OracleMajor: major, Schema: schema, Info: true})
		}},
		{"Alerts", func() (string, []any, error) {
			return AlertsStatement(AlertsOptions{OracleMajor: major, Schema: schema})
		}},
	}
	if cols := schema["V_$CELL"]; len(cols) > 0 {
		available := textType(cols["CELL_TYPE"])
		cases = append(cases, struct {
			name      string
			selectSQL func() (string, []any, error)
		}{"ConnectedCells", func() (string, []any, error) {
			return ConnectedCellsStatement(ConnectedCellsOptions{OracleMajor: major, Limit: 2, IncludeType: available, CellTypeKnownAbsent: !available})
		}})
	}
	for _, c := range cases {
		text, args, err := c.selectSQL()
		state := "selected"
		if err != nil {
			state = "unsupported_without_execution"
		}
		b, _ := json.Marshal(struct {
			Package, Fact                string
			Major                        int
			SQL                          string
			Args                         []any
			State, CapabilityProofSHA256 string
		}{"engineeredsystem/catalog", c.name, major, text, args, state, proofSHA})
		t.Log(string(b))
	}
}
