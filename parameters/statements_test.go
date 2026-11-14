package parameters_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/parameters"
)

func TestEffectiveSelectionBindsExactCurrentScope(t *testing.T) {
	t.Parallel()
	names := []string{"processes", "x') OR 1=1 --"}
	instances := []int64{1, 7}
	for _, major := range []int{11, 12, 19, 23} {
		t.Run(string(rune('A'+major)), func(t *testing.T) {
			sql, args, err := parameters.EffectiveStatement(major, parameters.Filter{Names: names, Instances: instances})
			if err != nil || !reflect.DeepEqual(args, []any{int64(1), int64(7), names[0], names[1]}) {
				t.Fatal(sql, args, err)
			}
			for _, clause := range []string{"FROM GV$SYSTEM_PARAMETER2 p", "p.TYPE AS TYPE", "p.ORDINAL AS ORDINAL", "p.VALUE AS VALUE", "p.DISPLAY_VALUE AS DISPLAY_VALUE", "p.ISDEFAULT AS ISDEFAULT", "p.INST_ID IN (:1,:2)", "NLSSORT(p.NAME, 'NLS_SORT=BINARY') IN (NLSSORT(:3, 'NLS_SORT=BINARY'),NLSSORT(:4, 'NLS_SORT=BINARY'))"} {
				if !strings.Contains(sql, clause) {
					t.Fatalf("missing %s", clause)
				}
			}
			if strings.Contains(sql, names[1]) || strings.Contains(sql, "COALESCE") || strings.Contains(sql, "TO_CHAR") || strings.Contains(sql, "FETCH") || strings.Contains(sql, "ROWNUM") {
				t.Fatal("data normalized, interpolated or truncated")
			}
			if strings.Contains(sql, "p.CON_ID AS CON_ID") != (major >= 12) || strings.Contains(sql, "p.ISPDB_MODIFIABLE AS ISPDB_MODIFIABLE") != (major >= 12) {
				t.Fatal("column version boundary lost")
			}
			if major >= 12 && !strings.Contains(sql, "p.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))") {
				t.Fatal("cross-container scope widened")
			}
			args[2] = "mutated"
			if names[0] != "processes" || instances[0] != 1 {
				t.Fatal("filter ownership lost")
			}
		})
	}
}
func TestSPFileSelectionPreservesDormantAndWildcard(t *testing.T) {
	t.Parallel()
	filter := parameters.SPFileFilter{Names: []string{"control_files"}, Instances: []int64{2}, SIDs: []string{"DormantCase"}}
	sql, args, err := parameters.SPFileStatement(19, filter)
	if err != nil || !reflect.DeepEqual(args, []any{int64(2), "control_files", "DormantCase", "*"}) {
		t.Fatal(sql, args, err)
	}
	if len(filter.SIDs) != 1 {
		t.Fatal("caller filter mutated")
	}
	for _, clause := range []string{"FROM GV$SPPARAMETER p", "p.SID AS SID", "p.TYPE AS TYPE", "p.ISSPECIFIED AS ISSPECIFIED", "NLSSORT(p.SID, 'NLS_SORT=BINARY') IN (NLSSORT(:3, 'NLS_SORT=BINARY'),NLSSORT(:4, 'NLS_SORT=BINARY'))"} {
		if !strings.Contains(sql, clause) {
			t.Fatal("stored source semantics lost", clause)
		}
	}
	if strings.Contains(sql, "JOIN") || strings.Contains(sql, "ISSPECIFIED=") || strings.Contains(sql, "GV$INSTANCE") || strings.Contains(sql, "TO_NUMBER(p.TYPE") {
		t.Fatal("stored applicability/native type inferred")
	}
	all, args, err := parameters.SPFileStatement(11, parameters.SPFileFilter{})
	if err != nil || len(args) != 0 || strings.Contains(all, " WHERE ") || strings.Contains(all, "CON_ID") || strings.Contains(all, "SID IN") {
		t.Fatal("all stored/dormant clauses narrowed", all, args, err)
	}
}
func TestParameterFilterValidation(t *testing.T) {
	t.Parallel()
	for name, filter := range map[string]parameters.Filter{
		"empty-name": {Names: []string{""}}, "padded": {Names: []string{" x"}}, "control": {Names: []string{"x\x00"}},
		"bad-utf8": {Names: []string{string([]byte{255})}}, "long": {Names: []string{strings.Repeat("x", 81)}},
		"too-many": {Names: make([]string, 1001)}, "zero-id": {Instances: []int64{0}}, "negative-id": {Instances: []int64{-1}},
	} {
		t.Run(name, func(t *testing.T) {
			sql, args, err := parameters.EffectiveStatement(19, filter)
			if err == nil || sql != "" || args != nil {
				t.Fatal("invalid filter accepted")
			}
		})
	}
	sids := make([]string, 1000)
	for i := range sids {
		sids[i] = "I"
	}
	if sql, args, err := parameters.SPFileStatement(19, parameters.SPFileFilter{SIDs: sids}); err == nil || sql != "" || args != nil {
		t.Fatal("wildcard exceeded Oracle IN budget")
	}
	sids[0] = "*"
	if _, args, err := parameters.SPFileStatement(19, parameters.SPFileFilter{SIDs: sids}); err != nil || len(args) != 1000 {
		t.Fatal("boundary including wildcard rejected", err)
	}
	if _, _, err := parameters.EffectiveStatement(10, parameters.Filter{}); err == nil {
		t.Fatal("unsupported source accepted")
	}
}
