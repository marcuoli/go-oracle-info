package parameters

import (
	"math/big"
	"strconv"
	"strings"
)

// The instance view's documented numeric TYPE is authoritative for the same
// named parameter. SPFILE.TYPE is opaque VARCHAR2 metadata, not a guessed enum.
// Strings/files compare equal only byte-for-byte; differing syntax/case may have
// parameter/platform-specific equivalence and therefore remains unresolved.
func comparePersistenceValue(kind int64, current, stored string) (bool, bool) {
	switch kind {
	case 1:
		a, okA := persistenceBoolean(current)
		b, okB := persistenceBoolean(stored)
		return a == b, okA && okB
	case 3, 6:
		a, okA := persistenceInteger(current, kind == 6)
		b, okB := persistenceInteger(stored, kind == 6)
		if !okA || !okB {
			return false, false
		}
		return a.Cmp(b) == 0, true
	case 2, 4:
		if current == stored {
			return true, true
		}
		return false, false
	default:
		return false, false
	}
}
func persistenceBoolean(text string) (bool, bool) {
	bytes := []byte(text)
	for i, c := range bytes {
		if c >= 'a' && c <= 'z' {
			bytes[i] = c - ('a' - 'A')
		} else if c < 'A' || c > 'Z' {
			return false, false
		}
	}
	switch string(bytes) {
	case "TRUE":
		return true, true
	case "FALSE":
		return false, true
	default:
		return false, false
	}
}

// Only reviewed literal integer and K/KB/M/MB/G/GB byte units are normalized.
// Expressions, decimal/scientific notation and unsupported units stay unknown.
// big.Int preserves values beyond floating-point and signed-64 precision.
func persistenceInteger(text string, units bool) (*big.Int, bool) {
	text = strings.Trim(text, " \t\r\n")
	end := 0
	if len(text) > 0 && (text[0] == '+' || text[0] == '-') {
		end++
	}
	start := end
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	if end == start {
		return nil, false
	}
	value, ok := new(big.Int).SetString(text[:end], 10)
	if !ok {
		return nil, false
	}
	suffix := strings.Trim(text[end:], " \t\r\n")
	power := 0
	if suffix != "" {
		if !units {
			return nil, false
		}
		switch suffix {
		case "K", "k", "KB", "kb", "Kb", "kB":
			power = 1
		case "M", "m", "MB", "mb", "Mb", "mB":
			power = 2
		case "G", "g", "GB", "gb", "Gb", "gB":
			power = 3
		default:
			return nil, false
		}
	}
	return value.Mul(value, new(big.Int).Lsh(big.NewInt(1), uint(power*10))), true
}
func persistenceOrdinal(text string) (int64, bool) {
	if text == "" || strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(text, 10, 64)
	return n, err == nil && n >= 0
}
