package catalog

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDatasetReleaseSelectionMatrix(t *testing.T) {
	cases := []struct {
		name      string
		selectSQL func(int) (string, []any, error)
	}{
		{name: "Metadata", selectSQL: func(major int) (string, []any, error) { return MetadataStatement(MetadataOptions{OracleMajor: major}) }},
		{name: "Configuration", selectSQL: func(major int) (string, []any, error) {
			return ConfigurationStatement(ConfigurationOptions{OracleMajor: major, Schema: Schema{"V_$CELL_CONFIG": {"CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "CLOB"}}})
		}},
		{name: "VisibleCellCount", selectSQL: func(major int) (string, []any, error) {
			return VisibleCellCountStatement(VisibleCellCountOptions{OracleMajor: major})
		}},
		{name: "Cells", selectSQL: func(major int) (string, []any, error) {
			return CellsStatement(CellsOptions{OracleMajor: major, Schema: Schema{"V_$CELL": {"CELL_HASHVAL": "NUMBER", "CELL_PATH": "VARCHAR2"}}})
		}},
		{name: "Alerts", selectSQL: func(major int) (string, []any, error) {
			return AlertsStatement(AlertsOptions{OracleMajor: major, Schema: Schema{"V_$CELL_OPEN_ALERTS": {"CELL_NAME": "VARCHAR2", "ALERT_VALUE": "CLOB"}}})
		}},
		{name: "ConnectedCells", selectSQL: func(major int) (string, []any, error) {
			return ConnectedCellsStatement(ConnectedCellsOptions{OracleMajor: major, Limit: 2, CellTypeKnownAbsent: true})
		}},
	}
	for _, c := range cases {
		for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
			text, args, err := c.selectSQL(major)
			if err == nil {
				encoded, _ := json.Marshal(struct {
					Package string
					Fact    string
					Major   int
					SQL     string
					Args    []any
				}{"engineeredsystem/catalog", c.name, major, text, args})
				t.Log(string(encoded))
			}
			admitted := major == 19 || major == 21 || major == 23 || major == 12 || major == 18
			if admitted {
				if err != nil || text == "" {
					t.Fatal(c.name, major, text, err)
				}
			} else if !errors.Is(err, ErrUnsupportedVersion) || text != "" {
				t.Fatal(c.name, major, text, err)
			}
		}
	}
}
