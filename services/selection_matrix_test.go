package services

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
		{name: "Names", selectSQL: func(major int) (string, []any, error) { return NamesStatement(NamesOptions{OracleMajor: major}) }},
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
				}{"services", c.name, major, text, args})
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
