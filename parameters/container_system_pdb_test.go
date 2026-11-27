package parameters_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestContainerSystemPDBReleaseSelection(t *testing.T) {
	const query = "SELECT p.INST_ID AS INST_ID, p.CON_ID AS CON_ID, p.NAME AS NAME, p.VALUE AS VALUE, p.DISPLAY_VALUE AS DISPLAY_VALUE FROM GV$SYSTEM_PARAMETER p WHERE p.CON_ID > 2 ORDER BY INST_ID, p.CON_ID, NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
	for _, test := range []struct {
		version   string
		supported bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false},
		{"12.1.0.1.0", true}, {"12.1.0.2.0", true}, {"12.2.0.1.0", true},
		{"18.4.0.0.0", true}, {"19.32.0.0.0", true}, {"21.3.0.0.0", true},
		{"23.9.0.25.07", true}, {"23.26.4.0.0", true}, {"26.4.0.0.0", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			options := parameters.ContainerSystemOptions{
				OracleVersion: test.version, CurrentContainerID: 1, Source: parameters.ContainerSystemPDBGlobal,
			}
			sql, args, err := parameters.ContainerSystemStatement(options)
			if !test.supported {
				if !errors.Is(err, parameters.ErrUnsupportedVersion) || sql != "" || args != nil {
					t.Fatalf("unsupported release produced SQL: %s %+v %v", sql, args, err)
				}
				return
			}
			if err != nil || sql != query || len(args) != 0 {
				t.Fatalf("unexpected PDB projection: %s %+v %v", sql, args, err)
			}
			facts, err := parameters.DecodeContainerSystem(containerSystemFixture(), options)
			if err != nil || len(facts.Rows) != 3 || !facts.Rows[1].Value.Null || facts.Provenance.Complete ||
				facts.Provenance.Source != "GV$SYSTEM_PARAMETER" || facts.Provenance.Scope != "root_visible_pdb_rows" {
				t.Fatalf("lost scoped evidence: %+v %v", facts, err)
			}
		})
	}
}

func TestContainerSystemPDBScope(t *testing.T) {
	options := parameters.ContainerSystemOptions{OracleVersion: "19.32.0.0.0", Source: parameters.ContainerSystemPDBGlobal}
	for _, test := range []struct {
		name      string
		connected int64
		ids       []int64
		want      error
	}{
		{"non-CDB", 0, nil, parameters.ErrContainerSystemUnsupportedScope},
		{"CDB seed", 2, nil, parameters.ErrContainerSystemSeedScope},
		{"CDB baseline", 1, []int64{0}, parameters.ErrInvalidFilter},
		{"CDB root", 1, []int64{1}, parameters.ErrInvalidFilter},
		{"CDB seed filter", 1, []int64{2}, parameters.ErrInvalidFilter},
		{"different PDB", 3, []int64{9}, parameters.ErrInvalidFilter},
		{"direct PDB", 3, nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			options.CurrentContainerID, options.ContainerIDs = test.connected, test.ids
			sql, args, err := parameters.ContainerSystemStatement(options)
			if !errors.Is(err, test.want) {
				t.Fatalf("scope: %v", err)
			}
			if test.want != nil {
				if sql != "" || args != nil {
					t.Fatalf("rejected scope selected SQL: %s %+v", sql, args)
				}
				return
			}
			if !strings.Contains(sql, "WHERE p.CON_ID > 2 AND p.CON_ID IN (:1)") || !reflect.DeepEqual(args, []any{int64(3)}) {
				t.Fatalf("direct connection widened scope: %s %+v", sql, args)
			}
		})
	}

	options.CurrentContainerID, options.ContainerIDs = 1, nil
	for _, bad := range []session.Value{{Text: "0"}, {Text: "1"}, {Text: "2"}, {Null: true}} {
		source := containerSystemFixture()
		source.Rows[2][1] = bad
		facts, err := parameters.DecodeContainerSystem(source, options)
		if !errors.Is(err, parameters.ErrInvalidEvidence) || facts.Rows != nil {
			t.Fatalf("excluded container %+v yielded a prefix: %+v %v", bad, facts, err)
		}
	}
}
