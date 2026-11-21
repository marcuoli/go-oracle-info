package projection

import (
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// FitsBufferedLimits checks row count and raw retained text bytes against
// limits already resolved by the caller. Scalar validation remains separate.
func FitsBufferedLimits(result session.Result, limits rowset.Limits) bool {
	if len(result.Rows) > limits.MaxRows {
		return false
	}
	remaining := limits.MaxBytes
	for _, row := range result.Rows {
		for _, value := range row {
			if len(value.Text) > remaining {
				return false
			}
			remaining -= len(value.Text)
		}
	}
	return true
}
