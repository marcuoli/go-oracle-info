// Package instances observes qualified instance visibility without admitting or
// connecting to members, or claiming complete physical RAC membership.
package instances

import (
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
)

// Provenance identifies only the admitted dynamic source and selected scope.
// Qualified visible rows do not prove every configured or reachable member.
type Provenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
}

var (
	// ErrUnsupportedVersion rejects a source below the reviewed 11g floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle instance visibility version")
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("instance reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed source metadata, IDs or values.
	ErrInvalidEvidence = errors.New("invalid Oracle instance visibility evidence")
)

type selection struct {
	sql        string
	schema     []rowset.Column
	provenance Provenance
}

// Statement selects all qualified visible GV$INSTANCE rows in the admitted
// container scope, including raw mounted/blocked states. It never filters to the
// connected instance or only open PDB members; reachability policy belongs outside.
func Statement(major int) (string, error) {
	plan, err := instancePlan(major)
	return plan.sql, err
}
func instancePlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	scope := "visible_qualified_instances"
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		scope = "current_container_and_whole_database_qualified_instances"
	}
	columns = append(columns, rowset.Column{Name: "INSTANCE_NUMBER", Kind: rowset.Integer})
	for _, name := range []string{"INSTANCE_NAME", "HOST_NAME", "STATUS", "DATABASE_STATUS", "PARALLEL", "BLOCKED"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "i." + column.Name + " AS " + column.Name
	}
	text := "SELECT " + strings.Join(aliases, ", ") + " FROM GV$INSTANCE i"
	if major >= 12 {
		text += " WHERE i.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))"
	}
	text += " ORDER BY i.INST_ID"
	if major >= 12 {
		text += ", i.CON_ID"
	}
	text += ", i.INSTANCE_NUMBER"
	return selection{sql: text, schema: columns, provenance: Provenance{Source: "GV$INSTANCE", OracleMajor: major, Scope: scope}}, nil
}
