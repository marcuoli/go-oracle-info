package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// LogicalDatabaseOptions carries the observed numeric release of the admitted handle.
type LogicalDatabaseOptions struct {
	OracleVersion string
}

// LogicalDatabaseRequest supplies the admitted options and caller-owned reference.
type LogicalDatabaseRequest struct {
	Options   LogicalDatabaseOptions
	Reference reference.Reference
}

// LogicalDatabaseRow preserves the database control-file identity as seen from
// the admitted connection. On a CDB this is the parent database's identity.
// CDB is absent before 12c. A nonnil CDB value can itself be SQL NULL.
type LogicalDatabaseRow struct {
	DBID session.Value
	Name session.Value
	CDB  *session.Value
}

// LogicalDatabaseResult has one source row or a successful empty row slice.
// Read and JSON mark completed transport. Decode validates supplied rows only.
type LogicalDatabaseResult struct {
	Provenance DatasetProvenance
	Rows       []LogicalDatabaseRow
}

var (
	// ErrLogicalDatabaseInaccessible marks explicit Oracle privilege rejection.
	ErrLogicalDatabaseInaccessible = projection.ErrSourceInaccessible

	// ErrLogicalDatabaseUnavailable marks an unresolved or unavailable source.
	ErrLogicalDatabaseUnavailable = projection.ErrSourceUnavailable
)

const logicalDatabaseProjection = "SELECT TO_CHAR(d.DBID,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS DBID, d.NAME AS NAME"

// LogicalDatabaseStatement selects the reviewed V$DATABASE projection for the
// observed release. Its DBID and name describe the database control file.
func LogicalDatabaseStatement(options LogicalDatabaseOptions) (string, []any, error) {
	p, err := logicalDatabasePlan(options)
	return p.sql, p.args, err
}

func logicalDatabasePlan(options LogicalDatabaseOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: logicalDatabaseProjection,
		schema: []rowset.Column{
			{Name: "DBID", Kind: rowset.Text},
			{Name: "NAME", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{
			Source: "V$DATABASE", OracleMajor: major, Scope: "controlfile_database_identity",
		},
	}
	if major >= 12 {
		p.sql += ", d.CDB AS CDB"
		p.schema = append(p.schema, rowset.Column{Name: "CDB", Kind: rowset.Text})
	}
	p.sql += " FROM V$DATABASE d"
	return p, nil
}

// DecodeLogicalDatabase validates the selected columns and scalar cardinality.
func DecodeLogicalDatabase(result session.Result, options LogicalDatabaseOptions) (LogicalDatabaseResult, error) {
	p, err := logicalDatabasePlan(options)
	if err != nil {
		return LogicalDatabaseResult{}, err
	}
	return decodeLogicalDatabase(result, p)
}

func decodeLogicalDatabase(result session.Result, p datasetSelection) (LogicalDatabaseResult, error) {
	index, err := datasetColumns(result, p, "DBID")
	if err != nil || len(result.Rows) > 1 {
		return LogicalDatabaseResult{}, ErrInvalidEvidence
	}
	rows := make([]LogicalDatabaseRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = LogicalDatabaseRow{DBID: r[index["DBID"]], Name: r[index["NAME"]]}
		if column, present := index["CDB"]; present {
			value := r[column]
			rows[i].CDB = &value
		}
	}
	return LogicalDatabaseResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadLogicalDatabase executes once through the admitted bounded reader.
// Query, decode and cancellation failures return no successful facts.
func ReadLogicalDatabase(ctx context.Context, reader Reader, request LogicalDatabaseRequest) (LogicalDatabaseResult, error) {
	p, err := logicalDatabasePlan(request.Options)
	if err != nil {
		return LogicalDatabaseResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return LogicalDatabaseResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return LogicalDatabaseResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return LogicalDatabaseResult{}, projection.ClassifySourceError(err)
	}
	facts, err := decodeLogicalDatabase(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return LogicalDatabaseResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// LogicalDatabaseRows returns the caller-native cursor. The caller owns
// iteration, close, handle affinity and streaming limits.
func LogicalDatabaseRows(ctx context.Context, q rowset.Queryer, options LogicalDatabaseOptions) (*sql.Rows, error) {
	p, err := logicalDatabasePlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, projection.ClassifySourceError(&DatasetError{p.provenance.Source, err})
	}
	return rows, nil
}

// LogicalDatabaseJSON independently consumes and validates one bounded cursor.
// A failed query, cursor, validation, encoding or budget check returns nil JSON.
func LogicalDatabaseJSON(ctx context.Context, q rowset.Queryer, options LogicalDatabaseOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := logicalDatabasePlan(options)
	if err != nil {
		return nil, err
	}
	data, err := readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeLogicalDatabase(result, p)
		return err
	})
	return data, projection.ClassifySourceError(err)
}
