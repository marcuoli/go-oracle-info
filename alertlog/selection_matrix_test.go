package alertlog

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOperationalSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		for idx, selectSQL := range []func() (string, []any, error){func() (string, []any, error) {
			return EntriesStatement(EntriesOptions{OracleMajor: major, Limit: 2, WindowSeconds: 60})
		}, func() (string, []any, error) {
			return CountsStatement(CountsOptions{OracleMajor: major, WindowSeconds: 60})
		}} {
			s, args, err := selectSQL()
			if err == nil {
				encoded, _ := json.Marshal(struct {
					Package string
					Fact    string
					Major   int
					SQL     string
					Args    []any
				}{"alertlog", []string{"Entries", "Counts"}[idx], major, s, args})
				t.Log(string(encoded))
			}
			admitted := major == 19 || major == 21 || major == 23
			if admitted {
				if err != nil || s == "" {
					t.Fatal(major, s, err)
				}
			} else if !errors.Is(err, ErrUnsupportedVersion) || s != "" {
				t.Fatal(major, s, err)
			}
		}
	}
}
