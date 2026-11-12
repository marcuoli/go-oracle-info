package catalog

import (
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
)

// DecodeSchema consumes complete caller-owned dictionary results. No discovered
// identifier becomes SQL; selectors remain confined to the reviewed view list.
func DecodeSchema(result session.Result) (Schema, error) {
	schema := make(Schema)
	if len(result.Columns) != 3 {
		return schema, errors.New("engineered-system capability metadata has an unexpected projection")
	}
	for _, row := range result.Rows {
		if len(row) != 3 || row[0].Null || row[1].Null || row[2].Null {
			return schema, errors.New("engineered-system capability metadata contains NULL identity")
		}
		view, name, kind := row[0].Text, row[1].Text, row[2].Text
		if schema[view] == nil {
			schema[view] = make(map[string]string)
		}
		if prior, exists := schema[view][name]; exists && prior != kind {
			return schema, errors.New("engineered-system capability metadata conflicts")
		}
		schema[view][name] = kind
	}
	return schema, ValidateSchema(schema)
}
