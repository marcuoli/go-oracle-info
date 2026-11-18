package objects

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
		{name: "InvalidObjects", selectSQL: func(major int) (string, []any, error) {
			return InvalidObjectsStatement(InvalidObjectsOptions{OracleMajor: major, Owners: []string{"CaseOwner"}})
		}},
		{name: "ObjectInventory", selectSQL: func(major int) (string, []any, error) {
			return ObjectInventoryStatement(ObjectInventoryOptions{OracleMajor: major, Owners: []string{"CaseOwner"}})
		}},
		{name: "Schema", selectSQL: func(major int) (string, []any, error) {
			return SchemaStatement(SchemaOptions{OracleMajor: major, Owner: "CaseOwner"})
		}},
		{name: "ObjectTotals", selectSQL: func(major int) (string, []any, error) {
			return ObjectTotalsStatement(ObjectTotalsOptions{OracleMajor: major, Owner: "CaseOwner"})
		}},
		{name: "Tables", selectSQL: func(major int) (string, []any, error) {
			return TablesStatement(TablesOptions{OracleMajor: major, Owner: "CaseOwner"})
		}},
		{name: "Table", selectSQL: func(major int) (string, []any, error) {
			return TableStatement(TableOptions{OracleMajor: major, Owner: "CaseOwner", Table: "T雪"})
		}},
		{name: "TableCount", selectSQL: func(major int) (string, []any, error) {
			return TableCountStatement(TableCountOptions{OracleMajor: major, Owner: "SYS", Table: "DUAL"})
		}},
		{name: "TablesStatistics", selectSQL: func(major int) (string, []any, error) {
			return TablesStatisticsStatement(TablesStatisticsOptions{OracleMajor: major, Owner: "CaseOwner"})
		}},
		{name: "TableStatistics", selectSQL: func(major int) (string, []any, error) {
			return TableStatisticsStatement(TableStatisticsOptions{OracleMajor: major, Owner: "CaseOwner", Table: "T雪"})
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
				}{"objects", c.name, major, text, args})
				t.Log(string(encoded))
			}
			admitted := major == 19 || major == 21 || major == 23
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
