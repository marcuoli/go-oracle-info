package grants

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ErrRoleCatalogUnavailable means the numeric release does not qualify
// the Oracle-maintained role flag.
var ErrRoleCatalogUnavailable = errors.New("Oracle role maintenance catalog is unavailable for this release")

// RoleCatalogRequest configures a complete read from the admitted dictionary.
type RoleCatalogRequest struct {
	OracleRelease string
	Reference     reference.Reference
	Limits        rowset.Limits
}

// RoleCatalogProvenance identifies the admitted numeric release and visibility.
type RoleCatalogProvenance struct {
	Source        string `json:"source"`
	OracleMajor   int    `json:"oracle_major"`
	OracleRelease string `json:"oracle_release"`
	Scope         string `json:"scope"`
}

// RoleCatalogEntry preserves the dictionary spelling and its Y, N or SQL NULL flag.
type RoleCatalogEntry struct {
	Role             session.Value
	OracleMaintained session.Value
}

// RoleCatalogResult holds roles visible in the connected dictionary. A successful
// empty result represents zero visible rows. No grant attribution is implied.
type RoleCatalogResult struct {
	Provenance RoleCatalogProvenance
	Records    []RoleCatalogEntry
}

var roleCatalog = selection{
	sql: "SELECT r.ROLE AS ROLE, r.ORACLE_MAINTAINED AS ORACLE_MAINTAINED " +
		"FROM DBA_ROLES r ORDER BY NLSSORT(r.ROLE, 'NLS_SORT=BINARY')",
	schema: []rowset.Column{
		{Name: "ROLE", Kind: rowset.Text},
		{Name: "ORACLE_MAINTAINED", Kind: rowset.Text},
	},
}

func catalogRoleProvenance(oracleRelease string) (RoleCatalogProvenance, error) {
	version, ok := oracleversion.Parse(oracleRelease)
	if !ok {
		return RoleCatalogProvenance{}, ErrInvalidRequest
	}
	major := int(version[0])
	// Normalize omitted numeric suffixes for the existing classified-grant gate.
	// Zero padding leaves an unknown 12c patch set below the supported boundary.
	qualifiedRelease := oracleRelease + strings.Repeat(".0", 4-strings.Count(oracleRelease, "."))
	release, err := classifiedRelease(ClassifiedOptions{
		Options:       Options{OracleMajor: major},
		OracleRelease: qualifiedRelease,
	})
	if errors.Is(err, ErrUnsupportedVersion) {
		return RoleCatalogProvenance{}, ErrRoleCatalogUnavailable
	}
	if err != nil {
		return RoleCatalogProvenance{}, err
	}
	if !release.roleMaintained {
		return RoleCatalogProvenance{}, ErrRoleCatalogUnavailable
	}
	return RoleCatalogProvenance{
		Source:        "DBA_ROLES",
		OracleMajor:   major,
		OracleRelease: oracleRelease,
		Scope:         "connected_container_dba_dictionary",
	}, nil
}

// RoleCatalogStatement selects the two observed dictionary columns with no binds.
func RoleCatalogStatement(oracleRelease string) (string, []any, error) {
	if _, err := catalogRoleProvenance(oracleRelease); err != nil {
		return "", nil, err
	}
	return roleCatalog.sql, nil, nil
}

func roleCatalogColumns(result session.Result) (map[string]int, error) {
	index, err := columns(result, roleCatalog)
	if err != nil {
		return nil, err
	}
	for _, row := range result.Rows {
		role, flag := row[index["ROLE"]], row[index["ORACLE_MAINTAINED"]]
		if role.Null || role.Text == "" || (!flag.Null && flag.Text != "Y" && flag.Text != "N") {
			return nil, ErrInvalidEvidence
		}
	}
	return index, nil
}

// DecodeRoleCatalog validates the projection and every row. The caller supplies
// the numeric release and owns proof that the result transport completed.
func DecodeRoleCatalog(result session.Result, oracleRelease string) (RoleCatalogResult, error) {
	provenance, err := catalogRoleProvenance(oracleRelease)
	if err != nil {
		return RoleCatalogResult{}, err
	}
	return decodeRoleCatalog(result, provenance)
}

func decodeRoleCatalog(result session.Result, provenance RoleCatalogProvenance) (RoleCatalogResult, error) {
	index, err := roleCatalogColumns(result)
	if err != nil {
		return RoleCatalogResult{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]RoleCatalogEntry, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = RoleCatalogEntry{Role: row[index["ROLE"]], OracleMaintained: row[index["ORACLE_MAINTAINED"]]}
	}
	return RoleCatalogResult{Provenance: provenance, Records: records}, nil
}

// ReadRoleCatalog reads the complete bounded catalog on a caller-owned reader.
func ReadRoleCatalog(ctx context.Context, reader Reader, request RoleCatalogRequest) (RoleCatalogResult, error) {
	provenance, err := catalogRoleProvenance(request.OracleRelease)
	if err != nil {
		return RoleCatalogResult{}, err
	}
	result, err := query(ctx, reader, Request{Reference: request.Reference, Limits: request.Limits}, roleCatalog)
	if err != nil {
		return RoleCatalogResult{}, err
	}
	facts, err := decodeRoleCatalog(result, provenance)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return RoleCatalogResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// RoleCatalogRows returns the native cursor. The caller owns iteration, limits,
// terminal error checks and close.
func RoleCatalogRows(ctx context.Context, q rowset.Queryer, oracleRelease string) (*sql.Rows, error) {
	if _, err := catalogRoleProvenance(oracleRelease); err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, roleCatalog.sql)
}

// RoleCatalogJSON reads and validates the whole catalog before returning JSON.
func RoleCatalogJSON(ctx context.Context, q rowset.Queryer, oracleRelease string, limits rowset.Limits) (json.RawMessage, error) {
	provenance, err := catalogRoleProvenance(oracleRelease)
	if err != nil {
		return nil, err
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, roleCatalog.sql), roleCatalog.schema, limits, rowset.EnvelopeOptions{
		Provenance: provenance,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := roleCatalogColumns(result)
			return err
		}),
	})
	return payload, projection.WrapColumnEnvelopeError(err, func(operation string, cause error) error {
		return &Error{Operation: operation, cause: cause}
	})
}
