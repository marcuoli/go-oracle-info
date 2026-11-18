package objects

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOldestSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		p, err := oldestDatasetPlan(StatisticsOptions{OracleMajor: major, Limit: 2}, []string{"SYSTEM"})
		if err == nil {
			encoded, _ := json.Marshal(struct {
				Package string
				Fact    string
				Major   int
				SQL     string
				Args    []any
			}{"objects", "OldestStatistics", major, p.sql, p.args})
			t.Log(string(encoded))
		}
		if major < 11 || major == 26 {
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatal(major, err)
			}
		} else if err != nil {
			t.Fatal(major, err)
		}
	}
}
