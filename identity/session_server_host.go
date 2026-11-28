package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SessionServerHostOptions carries the observed numeric Oracle release.
type SessionServerHostOptions struct {
	OracleVersion string
}

// SessionServerHostRequest supplies an admitted release and caller-owned reference.
type SessionServerHostRequest struct {
	Options   SessionServerHostOptions
	Reference reference.Reference
}

// SessionServerHostRow preserves the executing session's USERENV values,
// including SQL NULL and original spelling. The instance fields describe the
// same session as ServerHost and carry no independent membership claim.
type SessionServerHostRow struct {
	ServerHost     session.Value
	InstanceNumber session.Value
	InstanceName   session.Value
}

// SessionServerHostResult retains the selected source and its zero or one rows.
// Read and JSON attest complete consumption. Decode validates supplied evidence.
type SessionServerHostResult struct {
	Provenance DatasetProvenance
	Rows       []SessionServerHostRow
}

const sessionServerHostSQL = "SELECT SYS_CONTEXT('USERENV','SERVER_HOST',4000) AS SERVER_HOST, " +
	"SYS_CONTEXT('USERENV','INSTANCE') AS INSTANCE_NUMBER, " +
	"SYS_CONTEXT('USERENV','INSTANCE_NAME') AS INSTANCE_NAME FROM DUAL"

// SessionServerHostStatement selects the executing session's host and instance context.
func SessionServerHostStatement(options SessionServerHostOptions) (string, []any, error) {
	p, err := sessionServerHostPlan(options)
	return p.sql, p.args, err
}

func sessionServerHostPlan(options SessionServerHostOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 10 {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	if major == 10 {
		// Oracle first documents both SERVER_HOST and INSTANCE_NAME in 10.2.
		release, _ := oracleversion.Parse(options.OracleVersion)
		if release[1] < 2 {
			return datasetSelection{}, ErrUnsupportedVersion
		}
	}
	return datasetSelection{
		sql: sessionServerHostSQL,
		schema: []rowset.Column{
			{Name: "SERVER_HOST", Kind: rowset.Text},
			{Name: "INSTANCE_NUMBER", Kind: rowset.Text},
			{Name: "INSTANCE_NAME", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{
			Source: "SYS_CONTEXT(USERENV.SERVER_HOST)", OracleMajor: major,
			Scope: "executing_session_server_host",
		},
	}, nil
}

// DecodeSessionServerHost checks the selected schema and source cardinality.
func DecodeSessionServerHost(result session.Result, options SessionServerHostOptions) (SessionServerHostResult, error) {
	p, err := sessionServerHostPlan(options)
	if err != nil {
		return SessionServerHostResult{}, err
	}
	return decodeSessionServerHost(result, p)
}

func decodeSessionServerHost(result session.Result, p datasetSelection) (SessionServerHostResult, error) {
	index, err := datasetColumns(result, p, "INSTANCE_NUMBER")
	if err != nil || len(result.Rows) > 1 {
		return SessionServerHostResult{}, ErrInvalidEvidence
	}
	rows := make([]SessionServerHostRow, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = SessionServerHostRow{
			ServerHost:     row[index["SERVER_HOST"]],
			InstanceNumber: row[index["INSTANCE_NUMBER"]],
			InstanceName:   row[index["INSTANCE_NAME"]],
		}
	}
	return SessionServerHostResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSessionServerHost reads the executing session once through an admitted reader.
// Transport, cancellation and malformed evidence publish no successful rows.
func ReadSessionServerHost(ctx context.Context, reader Reader, request SessionServerHostRequest) (SessionServerHostResult, error) {
	p, err := sessionServerHostPlan(request.Options)
	if err != nil {
		return SessionServerHostResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return SessionServerHostResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return SessionServerHostResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionServerHostResult{}, err
	}
	facts, err := decodeSessionServerHost(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return SessionServerHostResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SessionServerHostRows returns the actual native cursor. The caller owns its
// iteration, close, session affinity and stream budget.
func SessionServerHostRows(ctx context.Context, q rowset.Queryer, options SessionServerHostOptions) (*sql.Rows, error) {
	p, err := sessionServerHostPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &DatasetError{Source: p.provenance.Source, cause: err}
	}
	return rows, nil
}

// SessionServerHostJSON independently consumes a bounded native cursor.
// A query, validation, cancellation, close or capacity failure returns nil JSON.
func SessionServerHostJSON(ctx context.Context, q rowset.Queryer, options SessionServerHostOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sessionServerHostPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeSessionServerHost(result, p)
		return err
	})
}
