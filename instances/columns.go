package instances

import (
	"strconv"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// columns validates the complete visible-instance source before either decoder
// publishes rows. Mapping, provenance and operation-specific errors stay with
// the caller; shared validation keeps visibility and version admission aligned.
func columns(result session.Result, schema []rowset.Column) (map[string]int, error) {
	names := make([]string, len(schema))
	for i, column := range schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, column := range schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return nil, ErrInvalidEvidence
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return nil, ErrInvalidEvidence
				}
			}
		}
		id := row[index["INST_ID"]]
		number, err := strconv.ParseInt(id.Text, 10, 64)
		if id.Null || err != nil || number < 1 {
			return nil, ErrInvalidEvidence
		}
	}
	return index, nil
}
