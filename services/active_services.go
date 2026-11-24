package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ActiveServicesOptions identifies the numeric release.
type ActiveServicesOptions struct {
	OracleVersion string
}

// ActiveServicesRequest carries caller admission, reference, and buffered limits.
type ActiveServicesRequest struct {
	Options   ActiveServicesOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// ActiveService preserves a service visible on a reporting instance. The row's
// ContainerID and SourceContainerID have independent meanings.
type ActiveService struct {
	InstanceID, Name, NetworkName  session.Value
	ContainerID, SourceContainerID *session.Value
}

// ActiveServicesResult retains selected rows and source provenance.
type ActiveServicesResult struct {
	Provenance DatasetProvenance
	Rows       []ActiveService
}

// ActiveServicesStatement selects the reviewed projection without truncation.
func ActiveServicesStatement(options ActiveServicesOptions) (string, error) {
	p, err := activeServicesPlan(options)
	return p.sql, err
}

func activeServicesPlan(options ActiveServicesOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 10 {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	p := datasetSelection{
		sql: "SELECT v.INST_ID AS INST_ID, v.NAME AS NAME, v.NETWORK_NAME AS NETWORK_NAME",
		schema: []rowset.Column{
			{Name: "INST_ID", Kind: rowset.Integer},
			{Name: "NAME", Kind: rowset.Text},
			{Name: "NETWORK_NAME", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "GV$ACTIVE_SERVICES", OracleMajor: major, Scope: "visible_active_services"},
	}
	if major >= 12 {
		p.sql += ", v.CON_ID AS CON_ID, TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer}, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM GV$ACTIVE_SERVICES v ORDER BY v.INST_ID, v.NAME"
	return p, nil
}

// DecodeActiveServices validates the complete supplied projection before returning any rows.
func DecodeActiveServices(result session.Result, options ActiveServicesOptions) (ActiveServicesResult, error) {
	p, err := activeServicesPlan(options)
	if err != nil {
		return ActiveServicesResult{}, err
	}
	return decodeActiveServices(result, p)
}

func decodeActiveServices(result session.Result, p datasetSelection) (ActiveServicesResult, error) {
	index, err := datasetColumns(result, p, "INST_ID")
	if err != nil {
		return ActiveServicesResult{}, err
	}
	rows := make([]ActiveService, len(result.Rows))
	for i, row := range result.Rows {
		item := ActiveService{InstanceID: row[index["INST_ID"]], Name: row[index["NAME"]], NetworkName: row[index["NETWORK_NAME"]]}
		instance, err := strconv.ParseInt(item.InstanceID.Text, 10, 64)
		if item.InstanceID.Null || err != nil || instance <= 0 {
			return ActiveServicesResult{}, ErrInvalidEvidence
		}
		if j, ok := index["CON_ID"]; ok {
			container, source := row[j], row[index["SOURCE_CON_ID"]]
			if (!container.Null && !numeric.IsUnsignedInteger(container.Text)) || source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return ActiveServicesResult{}, ErrInvalidEvidence
			}
			item.ContainerID, item.SourceContainerID = &container, &source
		}
		rows[i] = item
	}
	return ActiveServicesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadActiveServices executes once through the caller's admitted bounded reader.
func ReadActiveServices(ctx context.Context, reader Reader, request ActiveServicesRequest) (ActiveServicesResult, error) {
	p, err := activeServicesPlan(request.Options)
	if err != nil {
		return ActiveServicesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ActiveServicesResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ActiveServicesResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ActiveServicesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql, Args: p.args})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ActiveServicesResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return ActiveServicesResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeActiveServices(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return ActiveServicesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ActiveServicesRows transfers the caller-native cursor.
func ActiveServicesRows(ctx context.Context, q rowset.Queryer, options ActiveServicesOptions) (*sql.Rows, error) {
	p, err := activeServicesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// ActiveServicesJSON consumes and validates a separate complete bounded cursor.
func ActiveServicesJSON(ctx context.Context, q rowset.Queryer, options ActiveServicesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := activeServicesPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeActiveServices(result, p)
		return err
	})
}
