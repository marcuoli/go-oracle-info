package database_test

import (
	"testing"

	"github.com/marcuoli/go-oracle-info/database"
)

// These public projections serve different evidence contracts: changing display
// fallback or introducing a filter/default substitution would silently alter data.
func TestParameterProjectionCompatibility(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, got, want string }{
		{"existing display projection", database.ParametersSQL, "SELECT NAME, COALESCE(DISPLAY_VALUE,VALUE) AS VALUE FROM V$PARAMETER ORDER BY NAME"},
		{"native values and default flags", database.ParametersWithDefaultsSQL, "SELECT NAME AS NAME, VALUE AS VALUE, ISDEFAULT AS ISDEFAULT FROM V$PARAMETER ORDER BY NAME"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("SQL = %q, want frozen projection %q", tt.got, tt.want)
			}
		})
	}
}
