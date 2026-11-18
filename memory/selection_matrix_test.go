package memory

import (
	"errors"
	"testing"
)

func TestMemoryNativeSourceReleaseMatrix(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		o := Options{OracleMajor: major, Scope: NonCDB}
		_, err := sourcePlan(SGA, o, false)
		if major == 26 {
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatal(major, err)
			}
		} else if err != nil {
			t.Fatal(major, err)
		}
	}
}
