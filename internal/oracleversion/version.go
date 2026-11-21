// Package oracleversion parses observed numeric Oracle versions for source selection.
package oracleversion

import (
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
)

// Parse accepts two to five numeric components. Missing components are zero;
// selectors that need a patch-set boundary must require positive evidence for it.
func Parse(text string) ([5]uint64, bool) {
	var version [5]uint64
	if len(text) > 128 {
		return version, false
	}
	parts := strings.Split(text, ".")
	if len(parts) < 2 || len(parts) > len(version) {
		return version, false
	}
	for i, part := range parts {
		if !numeric.IsUnsignedInteger(part) {
			return [5]uint64{}, false
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return [5]uint64{}, false
		}
		version[i] = n
	}
	return version, true
}

// LegacyFactMajor admits the reviewed numeric releases for instance visibility
// and scalar system parameters. Oracle Database 26ai uses the numeric 23 series.
func LegacyFactMajor(text string) (int, bool) {
	numbers, ok := Parse(text)
	if !ok {
		return 0, false
	}
	switch numbers[0] {
	case 9:
		return 9, numbers[1] == 2
	case 10, 11, 12:
		return int(numbers[0]), numbers[1] == 1 || numbers[1] == 2
	case 18, 19, 21, 23:
		return int(numbers[0]), true
	default:
		return 0, false
	}
}
