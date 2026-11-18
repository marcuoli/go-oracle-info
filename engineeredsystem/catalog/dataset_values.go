package catalog

import (
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"strconv"
)

// ErrUnsupportedSource means the caller-admitted capability snapshot cannot
// select the requested fixed source. No discovery query or fallback is executed.
var ErrUnsupportedSource = errors.New("unsupported engineered-system source capability")

func validDatasetInstance(value session.Value) bool {
	if value.Null {
		return false
	}
	n, err := strconv.ParseInt(value.Text, 10, 64)
	return err == nil && n > 0
}

// Native LENGTH2/GETLENGTH uses UCS2 units. Preserve raw XML, including malformed
// XML evidence; this check establishes complete transport, not XML acceptance.
func completeDatasetPayload(payload, length session.Value) bool {
	if payload.Null {
		return length.Null
	}
	if length.Null {
		return false
	}
	want, err := strconv.ParseInt(length.Text, 10, 64)
	if err != nil || want < 0 {
		return false
	}
	actual := int64(0)
	for _, r := range payload.Text {
		actual++
		if r > 0xffff {
			actual++
		}
	}
	return actual == want
}

// CapabilitySchema builds a fresh capability snapshot from the complete metadata
// observation. It performs no SQL or caching, and revalidates caller-mutated rows.
func (facts MetadataResult) CapabilitySchema() (Schema, error) {
	result := session.Result{Columns: []string{"SOURCE_NAME", "COLUMN_NAME", "DATA_TYPE"}, Rows: make([][]session.Value, len(facts.Rows))}
	for i, r := range facts.Rows {
		result.Rows[i] = []session.Value{r.Source, r.Column, r.DataType}
	}
	return DecodeSchema(result)
}
