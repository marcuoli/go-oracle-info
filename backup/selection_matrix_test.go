package backup

import (
	"errors"
	"testing"
)

func TestOperationalSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		s, _, err := JobsStatement(JobsOptions{OracleMajor: major, Limit: 2})
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
