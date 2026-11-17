package projection

import (
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
	"unicode/utf8"
)

// TypedColumns validates the complete buffered shape and scalar representation.
// Oracle-specific identity, admission and error policy remain with each decoder.
func TypedColumns(result session.Result, schema []rowset.Column) (map[string]int, bool) {
	names := make([]string, len(schema))
	for i, column := range schema {
		names[i] = column.Name
	}
	index, ok := Columns(result, names...)
	if !ok {
		return nil, false
	}
	for _, row := range result.Rows {
		for _, column := range schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return nil, false
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return nil, false
				}
			}
		}
	}
	return index, true
}
