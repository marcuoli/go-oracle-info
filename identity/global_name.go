package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// GlobalNameOptions admits the observed numeric version of the caller's handle.
type GlobalNameOptions struct {
	OracleVersion string
}

// GlobalNameRequest supplies caller admission, a SQL reference, and buffered bounds.
type GlobalNameRequest struct {
	Options   GlobalNameOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// GlobalNameRow preserves the name reported by the connected dictionary.
// SourceContainerID is absent before 12c.
type GlobalNameRow struct {
	Name              session.Value
	SourceContainerID *session.Value
}

// GlobalNameResult retains the visible rows and their source provenance.
type GlobalNameResult struct {
	Provenance DatasetProvenance
	Rows       []GlobalNameRow
}

// GlobalNameStatement selects the reviewed source without SQL truncation.
func GlobalNameStatement(options GlobalNameOptions) (string, error) {
	p, err := globalNamePlan(options)
	return p.sql, err
}

func globalNamePlan(options GlobalNameOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.GLOBAL_NAME AS GLOBAL_NAME",
		schema: []rowset.Column{
			{Name: "GLOBAL_NAME", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "GLOBAL_NAME", OracleMajor: major, Scope: "connected_dictionary"},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.GLOBAL_NAME v"
	return p, nil
}

// DecodeGlobalName validates the supplied complete shape before returning any rows.
func DecodeGlobalName(result session.Result, options GlobalNameOptions) (GlobalNameResult, error) {
	p, err := globalNamePlan(options)
	if err != nil {
		return GlobalNameResult{}, err
	}
	return decodeGlobalName(result, p)
}

func decodeGlobalName(result session.Result, p datasetSelection) (GlobalNameResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil || len(result.Rows) > 1 {
		return GlobalNameResult{}, ErrInvalidEvidence
	}
	rows := make([]GlobalNameRow, len(result.Rows))
	for i, row := range result.Rows {
		item := GlobalNameRow{
			Name: row[index["GLOBAL_NAME"]],
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			value := row[j]
			if value.Null || !numeric.IsUnsignedInteger(value.Text) {
				return GlobalNameResult{}, ErrInvalidEvidence
			}
			item.SourceContainerID = &value
		}
		rows[i] = item
	}
	return GlobalNameResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadGlobalName executes once over the caller's admitted bounded reader.
func ReadGlobalName(ctx context.Context, reader Reader, request GlobalNameRequest) (GlobalNameResult, error) {
	p, err := globalNamePlan(request.Options)
	if err != nil {
		return GlobalNameResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return GlobalNameResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return GlobalNameResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return GlobalNameResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err = datasetReadError(ctx, p, err); err != nil {
		return GlobalNameResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return GlobalNameResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeGlobalName(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return GlobalNameResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// GlobalNameRows transfers the actual native cursor to its owner.
func GlobalNameRows(ctx context.Context, q rowset.Queryer, options GlobalNameOptions) (*sql.Rows, error) {
	p, err := globalNamePlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// GlobalNameJSON consumes a separate complete bounded cursor.
func GlobalNameJSON(ctx context.Context, q rowset.Queryer, options GlobalNameOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := globalNamePlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeGlobalName(r, p)
		return err
	})
}
