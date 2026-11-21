package identity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SessionContext retains source spelling, exact integer text and selected SQL NULL.
// Date/time labels are source-local civil values with no inferred timezone.
type SessionContext struct {
	ClientHost  session.Value
	OSUser      session.Value
	SessionUser session.Value
}

// SessionContextResult retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type SessionContextResult struct {
	Provenance Provenance
	Rows       []SessionContext
}

// SessionContextRequest admits the release/capabilities and a valid caller-owned SQL reference.
type SessionContextRequest struct {
	OracleMajor int
	Reference   reference.Reference
}

// SessionContextStatement selects SYS_CONTEXT(USERENV). The caller admits privileges, container and open state.
// No registration/flag is interpreted as licensing, reachability or acceptance.
func SessionContextStatement(major int) (string, error) {
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	return `SELECT SYS_CONTEXT('USERENV','HOST',4000) AS CLIENT_HOST, SYS_CONTEXT('USERENV','OS_USER',4000) AS OS_USER, SYS_CONTEXT('USERENV','SESSION_USER',4000) AS SESSION_USER FROM DUAL`, nil
}

func sessionContextSchema(major int) []rowset.Column {
	schema := []rowset.Column{
		{Name: "CLIENT_HOST", Kind: rowset.Text},
		{Name: "OS_USER", Kind: rowset.Text},
		{Name: "SESSION_USER", Kind: rowset.Text},
	}

	return schema
}

func sessionContextProvenance(major int) Provenance {
	p := Provenance{Source: "SYS_CONTEXT(USERENV)", Scope: "executing_session_client_context", OracleMajor: major}

	return p
}

// DecodeSessionContext validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func DecodeSessionContext(result session.Result, major int) (SessionContextResult, error) {
	if _, err := SessionContextStatement(major); err != nil {
		return SessionContextResult{}, err
	}
	index, err := factColumns(result, sessionContextSchema(major))
	if err != nil {
		return SessionContextResult{}, err
	}
	if len(result.Rows) > 1 {
		return SessionContextResult{}, ErrInvalidEvidence
	}
	facts := make([]SessionContext, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := SessionContext{
			ClientHost:  row[index["CLIENT_HOST"]],
			OSUser:      row[index["OS_USER"]],
			SessionUser: row[index["SESSION_USER"]],
		}

		facts = append(facts, fact)
	}

	return SessionContextResult{Provenance: sessionContextProvenance(major), Rows: facts}, nil
}

// ReadSessionContext executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func ReadSessionContext(ctx context.Context, reader Reader, request SessionContextRequest) (SessionContextResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return SessionContextResult{}, err
	}
	statement, err := SessionContextStatement(request.OracleMajor)
	if err != nil {
		return SessionContextResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return SessionContextResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return SessionContextResult{}, err
	}
	facts, err := DecodeSessionContext(result, request.OracleMajor)
	if err = factReadError(ctx, err); err != nil {
		return SessionContextResult{}, err
	}
	return facts, nil
}

// SessionContextRows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; SessionContextJSON independently owns its cursor.
func SessionContextRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	statement, err := SessionContextStatement(major)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// SessionContextJSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func SessionContextJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return SessionContextRows(ctx, q, major)
	}, sessionContextSchema(major), limits, rowset.EnvelopeOptions{
		MaxRows:    1,
		Provenance: sessionContextProvenance(major),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}
