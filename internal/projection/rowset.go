package projection

import (
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
	"unicode/utf8"
)

// FromRowset bridges an already consumed shared rowset into the domain decoder's
// exact text/NULL representation. It owns no cursor and performs no query or
// JSON round trip; callers validate domain scope before publishing the table.
func FromRowset(table rowset.Table) (session.Result, bool) {
	result := session.Result{Columns: make([]string, len(table.Columns)), Rows: make([][]session.Value, len(table.Rows))}
	for i, column := range table.Columns {
		if !utf8.ValidString(column.Name) || (column.Kind != rowset.Text && column.Kind != rowset.Integer) {
			return session.Result{}, false
		}
		result.Columns[i] = column.Name
	}
	for i, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return session.Result{}, false
		}
		result.Rows[i] = make([]session.Value, len(row))
		for j, value := range row {
			if value == nil {
				result.Rows[i][j].Null = true
				continue
			}
			switch table.Columns[j].Kind {
			case rowset.Text:
				text, ok := value.(string)
				if !ok || !utf8.ValidString(text) {
					return session.Result{}, false
				}
				result.Rows[i][j].Text = text
			case rowset.Integer:
				number, ok := value.(int64)
				if !ok {
					return session.Result{}, false
				}
				result.Rows[i][j].Text = strconv.FormatInt(number, 10)
			}
		}
	}
	return result, true
}
