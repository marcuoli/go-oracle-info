package projection

import (
	"github.com/marcuoli/go-oracle-session/rowset"
	"testing"
)

func TestFromRowsetRetainsExactTextIntegersAndNull(t *testing.T) {
	table := rowset.Table{Columns: []rowset.Column{{Name: "text", Kind: rowset.Text}, {Name: "integer", Kind: rowset.Integer}, {Name: "null", Kind: rowset.Text}}, Rows: [][]any{{"9223372036854775808", int64(-9223372036854775808), nil}}}
	result, ok := FromRowset(table)
	if !ok || result.Rows[0][0].Text != "9223372036854775808" || result.Rows[0][1].Text != "-9223372036854775808" || !result.Rows[0][2].Null {
		t.Fatal(result, ok)
	}
	table.Rows[0][0] = "changed"
	if result.Rows[0][0].Text != "9223372036854775808" {
		t.Fatal("result aliases mutable row")
	}
	for _, bad := range []rowset.Table{
		{Columns: []rowset.Column{{Name: "text", Kind: rowset.Text}}, Rows: [][]any{{int64(1)}}},
		{Columns: []rowset.Column{{Name: "integer", Kind: rowset.Integer}}, Rows: [][]any{{float64(1)}}},
		{Columns: []rowset.Column{{Name: "text", Kind: rowset.Text}}, Rows: [][]any{{}}},
	} {
		if got, ok := FromRowset(bad); ok || got.Rows != nil {
			t.Fatal("invalid rowset bridge accepted", got)
		}
	}
}
