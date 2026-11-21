package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// EncryptionKind selects one independently available dictionary observation.
// It never selects an operation that changes keys, wallets, files or encryption.
type EncryptionKind string

const (
	// TablespaceEncryption counts definitions marked encrypted and unknown flags.
	TablespaceEncryption EncryptionKind = "tablespaces"
	// ColumnEncryption counts rows in the encrypted-column dictionary.
	ColumnEncryption EncryptionKind = "columns"
)

// EncryptionRequest admits source version, connected CDB-root scope and one
// caller reference. Root selects CDB_ visibility, not proof of all-container
// access. Dictionary encryption requires OPEN; no mounted V$ fallback is used.
type EncryptionRequest struct {
	OracleMajor int
	CDBRoot     bool
	Kind        EncryptionKind
	Reference   reference.Reference
}

// EncryptionResult retains the exact count and unrecognized tablespace flags.
// A zero count describes only the selected visible dictionary; it never proves
// no encrypted data in closed/inaccessible containers or in external files.
type EncryptionResult struct {
	Provenance     Provenance
	EncryptedCount session.Value
	UnknownCount   session.Value
}

// EncryptionStatement selects a single static source. Keeping tablespaces and
// columns independent prevents a denied source from erasing valid peer facts.
func EncryptionStatement(major int, root bool, kind EncryptionKind) (string, error) {
	_, sql, err := encryptionPlan(major, root, kind)
	return sql, err
}

func encryptionPlan(major int, root bool, kind EncryptionKind) (Provenance, string, error) {
	if major < 11 {
		return Provenance{}, "", ErrUnsupportedVersion
	}
	if root && major < 12 {
		return Provenance{}, "", ErrInvalidRequest
	}
	prefix, scope := "DBA_", "current_container_dictionary"
	if root {
		prefix, scope = "CDB_", "visible_cdb_dictionary"
	}
	source, columns := "", ""
	switch kind {
	case TablespaceEncryption:
		source = prefix + "TABLESPACES"
		columns = "COUNT(CASE WHEN ENCRYPTED='YES' THEN 1 END) AS ENCRYPTED_COUNT, COUNT(CASE WHEN ENCRYPTED IS NULL OR ENCRYPTED NOT IN ('YES','NO') THEN 1 END) AS UNKNOWN_COUNT"
	case ColumnEncryption:
		source = prefix + "ENCRYPTED_COLUMNS"
		columns = "COUNT(*) AS ENCRYPTED_COUNT, 0 AS UNKNOWN_COUNT"
	default:
		return Provenance{}, "", ErrInvalidRequest
	}
	return Provenance{Source: source, Scope: scope, OracleMajor: major}, "SELECT " + columns + " FROM " + source, nil
}

func encryptionSchema() []rowset.Column {
	return []rowset.Column{{Name: "ENCRYPTED_COUNT", Kind: rowset.Integer}, {Name: "UNKNOWN_COUNT", Kind: rowset.Integer}}
}

// DecodeEncryption validates the whole scalar result, including cardinality.
// Pure decoding does not admit a connection, source visibility or open state.
func DecodeEncryption(result session.Result, major int, root bool, kind EncryptionKind) (EncryptionResult, error) {
	provenance, _, err := encryptionPlan(major, root, kind)
	if err != nil {
		return EncryptionResult{}, err
	}
	index, err := factColumns(result, encryptionSchema())
	if err != nil || len(result.Rows) != 1 {
		return EncryptionResult{}, ErrInvalidEvidence
	}
	row := result.Rows[0]
	for _, key := range []string{"ENCRYPTED_COUNT", "UNKNOWN_COUNT"} {
		n, err := strconv.ParseInt(row[index[key]].Text, 10, 64)
		if row[index[key]].Null || err != nil || n < 0 {
			return EncryptionResult{}, ErrInvalidEvidence
		}
	}
	return EncryptionResult{Provenance: provenance, EncryptedCount: row[index["ENCRYPTED_COUNT"]], UnknownCount: row[index["UNKNOWN_COUNT"]]}, nil
}

// ReadEncryption executes once on an already-admitted complete reader. Failed,
// malformed or post-query-cancelled reads publish no count; no source retry occurs.
func ReadEncryption(ctx context.Context, reader Reader, request EncryptionRequest) (EncryptionResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return EncryptionResult{}, err
	}
	_, sql, err := encryptionPlan(request.OracleMajor, request.CDBRoot, request.Kind)
	if err != nil {
		return EncryptionResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return EncryptionResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: sql})
	if err = factReadError(ctx, err); err != nil {
		return EncryptionResult{}, err
	}
	facts, err := DecodeEncryption(result, request.OracleMajor, request.CDBRoot, request.Kind)
	if err = factReadError(ctx, err); err != nil {
		return EncryptionResult{}, err
	}
	return facts, nil
}

// EncryptionRows returns the actual caller-owned cursor for the selected source.
// The caller owns Next/Scan/Err/Close, scope/grant admission and finite budgets.
func EncryptionRows(ctx context.Context, q rowset.Queryer, major int, root bool, kind EncryptionKind) (*sql.Rows, error) {
	_, sql, err := encryptionPlan(major, root, kind)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, sql)
}

// EncryptionJSON owns a separate cursor through session/rowset, checks complete
// domain/cardinality and includes provenance in its finite byte budget. No JSON
// prefix survives query, terminal, close, cancellation, shape or capacity failure.
func EncryptionJSON(ctx context.Context, q rowset.Queryer, major int, root bool, kind EncryptionKind, limits rowset.Limits) (json.RawMessage, error) {
	provenance, _, err := encryptionPlan(major, root, kind)
	if err != nil {
		return nil, err
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return EncryptionRows(ctx, q, major, root, kind)
	}, encryptionSchema(), limits, rowset.EnvelopeOptions{
		Provenance: provenance,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := DecodeEncryption(result, major, root, kind)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &ReadError{cause: cause}
	}, "limits", "header")
}

// EncryptionOptions carries the source selectors for the options-based fact APIs.
type EncryptionOptions struct {
	OracleMajor int
	Root        bool
	Kind        EncryptionKind
}

// EncryptionStatementWithOptions selects the same source as EncryptionStatement using one options value.
func EncryptionStatementWithOptions(options EncryptionOptions) (string, error) {
	return EncryptionStatement(options.OracleMajor, options.Root, options.Kind)
}

// DecodeEncryptionWithOptions selects the same source as DecodeEncryption using one options value.
func DecodeEncryptionWithOptions(result session.Result, options EncryptionOptions) (EncryptionResult, error) {
	return DecodeEncryption(result, options.OracleMajor, options.Root, options.Kind)
}

// EncryptionRowsWithOptions selects the same source as EncryptionRows using one options value.
func EncryptionRowsWithOptions(ctx context.Context, q rowset.Queryer, options EncryptionOptions) (*sql.Rows, error) {
	return EncryptionRows(ctx, q, options.OracleMajor, options.Root, options.Kind)
}

// EncryptionJSONWithOptions selects the same source as EncryptionJSON using one options value.
func EncryptionJSONWithOptions(ctx context.Context, q rowset.Queryer, options EncryptionOptions, limits rowset.Limits) (json.RawMessage, error) {
	return EncryptionJSON(ctx, q, options.OracleMajor, options.Root, options.Kind, limits)
}
