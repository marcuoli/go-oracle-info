// Package legacyversion selects the reviewed families that precede Oracle 11g.
package legacyversion

import (
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
)

// Major admits the observed numeric release, including 26ai's numeric 23 family.
func Major(version string) (int, bool) {
	if len(version) > 128 {
		return 0, false
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return 0, false
	}
	var numbers [5]uint64
	for i, part := range parts {
		if !numeric.IsUnsignedInteger(part) {
			return 0, false
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return 0, false
		}
		numbers[i] = n
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
