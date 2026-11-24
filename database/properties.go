package database

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

// PropertiesOptions admits the observed numeric version of the caller's handle.
type PropertiesOptions struct {
	OracleVersion string
}

// PropertiesRequest supplies caller admission, a SQL reference, and buffered bounds.
type PropertiesRequest struct {
	Options   PropertiesOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// Property preserves one source property, including its nullable value and description.
// SourceContainerID is absent before 12c.
type Property struct {
	Name              session.Value
	Value             session.Value
	Description       session.Value
	SourceContainerID *session.Value
}

// PropertiesResult retains the visible rows and their source provenance.
type PropertiesResult struct {
	Provenance DatasetProvenance
	Rows       []Property
}

// PropertiesStatement selects the reviewed source without SQL truncation.
func PropertiesStatement(options PropertiesOptions) (string, error) {
	p, err := propertiesPlan(options)
	return p.sql, err
}

func propertiesPlan(options PropertiesOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.PROPERTY_NAME AS PROPERTY_NAME, v.PROPERTY_VALUE AS PROPERTY_VALUE, v.DESCRIPTION AS DESCRIPTION",
		schema: []rowset.Column{
			{Name: "PROPERTY_NAME", Kind: rowset.Text},
			{Name: "PROPERTY_VALUE", Kind: rowset.Text},
			{Name: "DESCRIPTION", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "DATABASE_PROPERTIES", OracleMajor: major, Scope: "connected_dictionary"},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DATABASE_PROPERTIES v ORDER BY v.PROPERTY_NAME"
	return p, nil
}

// DecodeProperties validates the supplied complete shape before returning any rows.
func DecodeProperties(result session.Result, options PropertiesOptions) (PropertiesResult, error) {
	p, err := propertiesPlan(options)
	if err != nil {
		return PropertiesResult{}, err
	}
	return decodeProperties(result, p)
}

func decodeProperties(result session.Result, p datasetSelection) (PropertiesResult, error) {
	index, err := datasetColumns(result, p)
	if err != nil {
		return PropertiesResult{}, ErrInvalidEvidence
	}
	rows := make([]Property, len(result.Rows))
	for i, row := range result.Rows {
		item := Property{
			Name:        row[index["PROPERTY_NAME"]],
			Value:       row[index["PROPERTY_VALUE"]],
			Description: row[index["DESCRIPTION"]],
		}
		if item.Name.Text == "" {
			return PropertiesResult{}, ErrInvalidEvidence
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			value := row[j]
			if value.Null || !numeric.IsUnsignedInteger(value.Text) {
				return PropertiesResult{}, ErrInvalidEvidence
			}
			item.SourceContainerID = &value
		}
		rows[i] = item
	}
	return PropertiesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadProperties executes once over the caller's admitted bounded reader.
func ReadProperties(ctx context.Context, reader Reader, request PropertiesRequest) (PropertiesResult, error) {
	p, err := propertiesPlan(request.Options)
	if err != nil {
		return PropertiesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return PropertiesResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return PropertiesResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return PropertiesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err = datasetReadError(ctx, p, err); err != nil {
		return PropertiesResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return PropertiesResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeProperties(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return PropertiesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PropertiesRows transfers the actual native cursor to its owner.
func PropertiesRows(ctx context.Context, q rowset.Queryer, options PropertiesOptions) (*sql.Rows, error) {
	p, err := propertiesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// PropertiesJSON consumes a separate complete bounded cursor.
func PropertiesJSON(ctx context.Context, q rowset.Queryer, options PropertiesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := propertiesPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeProperties(r, p)
		return err
	})
}
