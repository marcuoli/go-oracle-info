package objects

import (
	"reflect"
	"strings"
	"testing"
)

func TestOwnerValuesStayBoundAndInputRemainsOwnedByCaller(t *testing.T) {
	for _, tc := range []struct {
		name      string
		statement func(...string) (string, []any)
	}{{"invalid", InvalidStatement}, {"inventory", InventoryStatement}} {
		t.Run(tc.name, func(t *testing.T) {
			owners := []string{"MiXeD", "X') OR 1=1 --", "N'雪"}
			before := append([]string(nil), owners...)
			text, args := tc.statement(owners...)
			if !strings.Contains(text, "OWNER IN (:1,:2,:3)") || len(args) != 3 {
				t.Fatal("owner list lost bound scope", text, args)
			}
			for i, owner := range owners {
				if args[i] != owner || strings.Contains(text, owner) {
					t.Fatal("owner was normalized or interpolated", i)
				}
			}
			args[0] = "changed"
			if !reflect.DeepEqual(owners, before) {
				t.Fatal("query arguments alias the caller's slice")
			}
			text, args = tc.statement()
			if strings.Contains(text, "OWNER IN") || len(args) != 0 || !strings.Contains(text, "OWNER, OBJECT_TYPE") {
				t.Fatal("all-visible inventory narrowed or lost owner")
			}
		})
	}
}

func TestCountIdentifiersAreQuotedWithoutNormalizingOrExecutingInput(t *testing.T) {
	for _, tc := range []struct{ name, owner, table, want string }{
		{"mixed", "App", "MixedTable", `"App"."MixedTable"`},
		{"quote", "A\"B", "X\"; DROP TABLE T; --", `"A""B"."X""; DROP TABLE T; --"`},
		{"unicode", "雪", "表", `"雪"."表"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := CountStatement(tc.owner, tc.table)
			if err != nil || !strings.HasSuffix(text, " FROM "+tc.want) {
				t.Fatal("identifier text escaped its quoted scope", text, err)
			}
			if !strings.Contains(text, "NLS_NUMERIC_CHARACTERS=''.,''") {
				t.Fatal("exact count depends on session locale")
			}
		})
	}
	for _, tc := range []struct{ name, value string }{
		{"empty", ""}, {"NUL", "A\x00B"}, {"invalid-utf8", string([]byte{0xff})}, {"too-long", strings.Repeat("X", 129)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if text, err := CountStatement(tc.value, "T"); err == nil || text != "" {
				t.Fatal("invalid owner was accepted")
			}
			if text, err := CountStatement("A", tc.value); err == nil || text != "" {
				t.Fatal("invalid table was accepted")
			}
		})
	}
}
