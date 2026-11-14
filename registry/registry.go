// Package registry selects and decodes exact current-container dictionary
// component facts. It owns no connections, grants, container changes or acceptance
// policy. Failed/incomplete reads must not be presented as an empty inventory.
package registry

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/session"
)

var (
	// ErrUnsupportedVersion rejects a source below the reviewed Oracle 11+ floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle registry version")
	// ErrInvalidEvidence rejects a projection or row shape that cannot be decoded.
	ErrInvalidEvidence = errors.New("invalid Oracle registry evidence")
)

// Component preserves source strings and SQL NULL without interpreting status.
// Modified is the raw VARCHAR2 value from DBA_REGISTRY, not a parsed timestamp or
// UTC time. VersionFull is nil when its column was not selected on a legacy source;
// a non-nil value with Valid false is a selected SQL NULL. Versions are not merged
// or padded into an inferred release update, and registration is not licensing.
type Component struct {
	ID          sql.NullString
	Name        sql.NullString
	Version     sql.NullString
	VersionFull *sql.NullString
	Status      sql.NullString
	Modified    sql.NullString
}

// Statement selects dictionary components visible in the admitted current
// container. VERSION_FULL is documented from 18 onward; 11 through 17 select only
// the legacy version. The caller admits privileges/open-state availability and
// bounds the complete read; this is not a cross-container or cluster inventory.
func Statement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	columns := "COMP_ID AS COMP_ID, COMP_NAME AS COMP_NAME, VERSION AS VERSION"
	if major >= 18 {
		columns += ", VERSION_FULL AS VERSION_FULL"
	}
	return "SELECT " + columns + ", STATUS AS STATUS, MODIFIED AS MODIFIED FROM DBA_REGISTRY ORDER BY COMP_ID", nil
}

// Decode validates the complete Statement projection before retaining any rows.
// All strings, NULLs, repeated IDs and source row order survive unchanged. A nil
// error and zero rows describe only a successful empty source; query, privilege,
// MOUNTED-state and cursor/capacity errors remain with the caller. A malformed late
// row returns no partial inventory. Decoding neither scores nor filters statuses.
func Decode(result session.Result, major int) ([]Component, error) {
	if _, err := Statement(major); err != nil {
		return nil, err
	}
	names := []string{"COMP_ID", "COMP_NAME", "VERSION", "STATUS", "MODIFIED"}
	if major >= 18 {
		names = append(names, "VERSION_FULL")
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	facts := make([]Component, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := Component{
			ID: nullable(row[index["COMP_ID"]]), Name: nullable(row[index["COMP_NAME"]]),
			Version: nullable(row[index["VERSION"]]), Status: nullable(row[index["STATUS"]]),
			Modified: nullable(row[index["MODIFIED"]]),
		}
		if major >= 18 {
			full := nullable(row[index["VERSION_FULL"]])
			fact.VersionFull = &full
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func nullable(value session.Value) sql.NullString {
	if value.Null {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.Clone(value.Text), Valid: true}
}
