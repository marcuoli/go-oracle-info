// Package projection validates complete result shapes before facts are decoded.
package projection

import (
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// Columns indexes a fixed projection, accepting case-insensitive metadata order.
// It rejects all malformed rows before a caller can publish a valid-looking prefix.
func Columns(result session.Result, names ...string) (map[string]int, bool) {
	if len(result.Columns) != len(names) {
		return nil, false
	}
	index := make(map[string]int, len(names))
	for i, name := range result.Columns {
		name = strings.ToUpper(name)
		if _, exists := index[name]; name == "" || exists {
			return nil, false
		}
		index[name] = i
	}
	for _, name := range names {
		if _, exists := index[name]; !exists {
			return nil, false
		}
	}
	for _, row := range result.Rows {
		if len(row) != len(names) {
			return nil, false
		}
	}
	return index, true
}
