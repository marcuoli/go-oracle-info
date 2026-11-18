package performance

import (
	"errors"
	"testing"
)

func TestOperationalSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		for _, selectSQL := range []func() (string, []any, error){func() (string, []any, error) { return BlockersStatement(BlockersOptions{OracleMajor: major}) }, func() (string, []any, error) {
			return ResourceLimitsStatement(ResourceLimitsOptions{OracleMajor: major})
		}} {
			s, _, err := selectSQL()
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
