package containers_test

import (
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/containers"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestPDBScopeProjectionsSelectOnlyAdmittedReleases(t *testing.T) {
	for _, tc := range []struct {
		version   string
		supported bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false},
		{"12.1.0.1.0", true}, {"12.1.0.2.0", true}, {"12.2.0.1.0", true},
		{"18.0.0.0.0", true}, {"19.32.0.0.0", true}, {"21.0.0.0.0", true},
		{"23.9.0.0.0", true}, {"23.26.1.0.0", true}, {"26ai", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			pdbSQL, pdbErr := containers.VisiblePDBsStatement(tc.version)
			sessionSQL, sessionErr := containers.SessionLocationStatement(tc.version)
			if !tc.supported {
				if pdbErr == nil || sessionErr == nil || pdbSQL != "" || sessionSQL != "" {
					t.Fatal("an unsupported release selected Oracle SQL")
				}
				return
			}
			if pdbErr != nil || sessionErr != nil || !strings.Contains(pdbSQL, "p.CON_ID > 2") ||
				!strings.Contains(pdbSQL, "SYS_CONTEXT('USERENV','CON_ID') = '1'") ||
				!strings.Contains(sessionSQL, "AS INSTANCE_ID") {
				t.Fatalf("scope or location guard missing: %v %v", pdbErr, sessionErr)
			}
		})
	}
	location, err := containers.DecodeSessionLocation(session.Result{
		Columns: []string{"INSTANCE_ID", "CON_ID", "CONTAINER"},
		Rows:    [][]session.Value{{{Text: "4"}, {Text: "7"}, {Text: "Exact_PDB"}}},
	}, "23.26.1.0.0")
	if err != nil || len(location) != 1 || location[0].Name.Text != "Exact_PDB" || location[0].InstanceID.Text != "4" || location[0].ContainerID.Text != "7" {
		t.Fatalf("location changed: %+v, %v", location, err)
	}
}
