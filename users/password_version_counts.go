package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ErrSourceInaccessible marks a privilege rejection by Oracle.
var ErrSourceInaccessible = projection.ErrSourceInaccessible

// ErrSourceUnavailable marks an unresolved or unavailable Oracle source.
var ErrSourceUnavailable = projection.ErrSourceUnavailable

// PasswordVersionCountsOptions carries the observed release of the admitted dictionary.
type PasswordVersionCountsOptions struct{ OracleVersion string }

// PasswordVersionCountsRequest supplies a caller reference and buffered bounds.
type PasswordVersionCountsRequest struct {
	Options   PasswordVersionCountsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// PasswordVersionCount preserves one raw listed version set and its exact
// positive account count. It identifies no account or password verifier value.
type PasswordVersionCount struct {
	Versions, AccountCount session.Value
}

// PasswordVersionClass describes reported verifier types without predicting login.
type PasswordVersionClass string

const (
	PasswordVersionsUnreported    PasswordVersionClass = "unreported"
	PasswordVersions10GOnly       PasswordVersionClass = "10g_only"
	PasswordVersionsCaseSensitive PasswordVersionClass = "case_sensitive"
	PasswordVersionsHTTPOnly      PasswordVersionClass = "http_only"
	PasswordVersionsUnknown       PasswordVersionClass = "unknown"
)

// Class interprets the documented version tokens. HTTP refers to a separate
// authentication mechanism. Unrecognized metadata remains unknown.
func (c PasswordVersionCount) Class() PasswordVersionClass {
	if c.Versions.Null {
		return PasswordVersionsUnreported
	}
	var legacy, modern, http bool
	for _, token := range strings.Fields(c.Versions.Text) {
		switch token {
		case "10G":
			legacy = true
		case "11G", "12C":
			modern = true
		case "HTTP":
			http = true
		default:
			return PasswordVersionsUnknown
		}
	}
	switch {
	case modern:
		return PasswordVersionsCaseSensitive
	case legacy:
		return PasswordVersions10GOnly
	case http:
		return PasswordVersionsHTTPOnly
	default:
		return PasswordVersionsUnknown
	}
}

// PasswordVersionCountsResult contains the selected connected dictionary groups.
type PasswordVersionCountsResult struct {
	Provenance Provenance
	Rows       []PasswordVersionCount
}

// PasswordVersionCountsStatement selects a histogram with no per-account fields.
func PasswordVersionCountsStatement(options PasswordVersionCountsOptions) (string, error) {
	plan, err := passwordVersionCountsPlan(options)
	return plan.sql, err
}

func passwordVersionCountsPlan(options PasswordVersionCountsOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok || major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	return selection{
		sql: "SELECT u.PASSWORD_VERSIONS AS PASSWORD_VERSIONS, " +
			"TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS ACCOUNT_COUNT " +
			"FROM SYS.DBA_USERS u GROUP BY u.PASSWORD_VERSIONS, NLSSORT(u.PASSWORD_VERSIONS, 'NLS_SORT=BINARY') " +
			"ORDER BY NLSSORT(u.PASSWORD_VERSIONS, 'NLS_SORT=BINARY') NULLS FIRST",
		schema:     []rowset.Column{{Name: "PASSWORD_VERSIONS", Kind: rowset.Text}, {Name: "ACCOUNT_COUNT", Kind: rowset.Text}},
		provenance: Provenance{Source: "DBA_USERS", OracleMajor: major, Scope: "connected_dictionary"},
	}, nil
}

// DecodePasswordVersionCounts validates the full histogram before returning it.
func DecodePasswordVersionCounts(result session.Result, options PasswordVersionCountsOptions) (PasswordVersionCountsResult, error) {
	plan, err := passwordVersionCountsPlan(options)
	if err != nil {
		return PasswordVersionCountsResult{}, err
	}
	return decodePasswordVersionCounts(result, plan)
}

func decodePasswordVersionCounts(result session.Result, plan selection) (PasswordVersionCountsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return PasswordVersionCountsResult{}, err
	}
	rows := make([]PasswordVersionCount, len(result.Rows))
	seen := make(map[session.Value]struct{}, len(rows))
	for i, row := range result.Rows {
		v := PasswordVersionCount{Versions: row[index["PASSWORD_VERSIONS"]], AccountCount: row[index["ACCOUNT_COUNT"]]}
		if v.AccountCount.Null || !numeric.IsUnsignedInteger(v.AccountCount.Text) ||
			!strings.ContainsAny(v.AccountCount.Text, "123456789") {
			return PasswordVersionCountsResult{}, ErrInvalidEvidence
		}
		if _, duplicate := seen[v.Versions]; duplicate {
			return PasswordVersionCountsResult{}, ErrInvalidEvidence
		}
		seen[v.Versions] = struct{}{}
		rows[i] = v
	}
	return PasswordVersionCountsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadPasswordVersionCounts executes once over a caller-admitted complete reader.
func ReadPasswordVersionCounts(ctx context.Context, reader Reader, request PasswordVersionCountsRequest) (PasswordVersionCountsResult, error) {
	plan, err := passwordVersionCountsPlan(request.Options)
	if err != nil {
		return PasswordVersionCountsResult{}, err
	}
	result, err := query(ctx, reader, Request{Reference: request.Reference, Limits: request.Limits}, plan)
	if err != nil {
		return PasswordVersionCountsResult{}, projection.ClassifySourceError(err)
	}
	facts, err := decodePasswordVersionCounts(result, plan)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return PasswordVersionCountsResult{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// PasswordVersionCountsRows transfers the actual caller-native cursor.
func PasswordVersionCountsRows(ctx context.Context, q rowset.Queryer, options PasswordVersionCountsOptions) (*sql.Rows, error) {
	plan, err := passwordVersionCountsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, projection.ClassifySourceError(&Error{Operation: "query", cause: err})
	}
	return rows, nil
}

// PasswordVersionCountsJSON consumes a separate complete bounded cursor.
func PasswordVersionCountsJSON(ctx context.Context, q rowset.Queryer, options PasswordVersionCountsOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := passwordVersionCountsPlan(options)
	if err != nil {
		return nil, err
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(r session.Result) error {
			_, err := decodePasswordVersionCounts(r, plan)
			return err
		}),
	})
	return payload, projection.WrapColumnEnvelopeError(err, func(operation string, cause error) error {
		failure := &Error{Operation: operation, cause: cause}
		if operation == "query" {
			return projection.ClassifySourceError(failure)
		}
		return failure
	})
}
