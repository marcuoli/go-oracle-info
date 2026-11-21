package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// InvalidObject adds source-local creation and last-DDL labels to invalid-object
// identity/status. It neither changes the published InvalidStatement nor treats
// these timezone-free civil labels as UTC instants.
type InvalidObject struct{ Owner, ObjectType, ObjectName, Status, Created, LastDDLTime session.Value }

// InvalidDetailsResult preserves complete selected order, duplicates and NULLs.
type InvalidDetailsResult struct {
	Provenance Provenance
	Rows       []InvalidObject
}

// InvalidDetailsRequest admits an exact owner list, release and execution reference.
// Empty Owners selects every dictionary-visible owner, not application schemas.
type InvalidDetailsRequest struct {
	OracleMajor int
	Owners      []string
	Reference   reference.Reference
}

// InvalidDetailsStatement extends the invalid-object responsibility with dates.
// Bound owners retain exact spelling; no internal-schema blacklist is inferred.
func InvalidDetailsStatement(major int, owners ...string) (string, []any, error) {
	if major < 11 {
		return "", nil, ErrUnsupportedVersion
	}
	var filter string
	var args []any
	if len(owners) > 0 {
		binds := make([]string, len(owners))
		for i, owner := range owners {
			args = append(args, owner)
			binds[i] = "NLSSORT(:" + strconv.Itoa(i+1) + ", 'NLS_SORT=BINARY')"
		}
		// Caller collation must not broaden an exact quoted-owner selection.
		filter = " WHERE NLSSORT(OWNER, 'NLS_SORT=BINARY') IN (" + strings.Join(binds, ",") + ")"
	}
	if filter == "" {
		filter = " WHERE STATUS='INVALID'"
	} else {
		filter += " AND STATUS='INVALID'"
	}
	return `SELECT OWNER AS OWNER, OBJECT_TYPE AS OBJECT_TYPE, OBJECT_NAME AS OBJECT_NAME, STATUS AS STATUS, TO_CHAR(CREATED,'YYYY-MM-DD"T"HH24:MI:SS') AS CREATED, TO_CHAR(LAST_DDL_TIME,'YYYY-MM-DD"T"HH24:MI:SS') AS LAST_DDL_TIME FROM DBA_OBJECTS` + filter + " ORDER BY OWNER,OBJECT_TYPE,OBJECT_NAME", args, nil
}

func invalidDetailsSchema() []rowset.Column {
	return []rowset.Column{{Name: "OWNER", Kind: rowset.Text}, {Name: "OBJECT_TYPE", Kind: rowset.Text}, {Name: "OBJECT_NAME", Kind: rowset.Text}, {Name: "STATUS", Kind: rowset.Text}, {Name: "CREATED", Kind: rowset.Text}, {Name: "LAST_DDL_TIME", Kind: rowset.Text}}
}

func invalidDetailsProvenance(major int, owners []string) Provenance {
	return Provenance{Source: "DBA_OBJECTS", Scope: "current_container_invalid_objects", OracleMajor: major, Owners: append([]string(nil), owners...)}
}

// DecodeInvalidDetails validates every projected row before returning facts.
// It refuses out-of-selection owners/statuses rather than blessing wrong evidence.
func DecodeInvalidDetails(result session.Result, major int, owners ...string) (InvalidDetailsResult, error) {
	if _, _, err := InvalidDetailsStatement(major, owners...); err != nil {
		return InvalidDetailsResult{}, err
	}
	index, err := factColumns(result, invalidDetailsSchema())
	if err != nil {
		return InvalidDetailsResult{}, err
	}
	facts := make([]InvalidObject, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := InvalidObject{Owner: row[index["OWNER"]], ObjectType: row[index["OBJECT_TYPE"]], ObjectName: row[index["OBJECT_NAME"]], Status: row[index["STATUS"]], Created: row[index["CREATED"]], LastDDLTime: row[index["LAST_DDL_TIME"]]}
		if fact.Status.Null || fact.Status.Text != "INVALID" {
			return InvalidDetailsResult{}, ErrInvalidEvidence
		}
		if len(owners) > 0 {
			matched := false
			for _, owner := range owners {
				matched = matched || (!fact.Owner.Null && fact.Owner.Text == owner)
			}
			if !matched {
				return InvalidDetailsResult{}, ErrInvalidEvidence
			}
		}
		facts = append(facts, fact)
	}
	return InvalidDetailsResult{Provenance: invalidDetailsProvenance(major, owners), Rows: facts}, nil
}

// ReadInvalidDetails queries once and retains no failed/cancelled/malformed prefix.
func ReadInvalidDetails(ctx context.Context, reader Reader, request InvalidDetailsRequest) (InvalidDetailsResult, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return InvalidDetailsResult{}, err
	}
	statement, args, err := InvalidDetailsStatement(request.OracleMajor, request.Owners...)
	if err != nil {
		return InvalidDetailsResult{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return InvalidDetailsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement, Args: args})
	if err = factReadError(ctx, err); err != nil {
		return InvalidDetailsResult{}, err
	}
	facts, err := DecodeInvalidDetails(result, request.OracleMajor, request.Owners...)
	if err = factReadError(ctx, err); err != nil {
		return InvalidDetailsResult{}, err
	}
	return facts, nil
}

// InvalidDetailsRows returns the caller's actual native cursor with bound owners.
// Callers own iteration/Err/Close and their declared complete-read budgets.
func InvalidDetailsRows(ctx context.Context, q rowset.Queryer, major int, owners ...string) (*sql.Rows, error) {
	statement, args, err := InvalidDetailsStatement(major, owners...)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement, args...)
}

// InvalidDetailsJSON independently finalizes its cursor before publishing bounded
// JSON. Native numeric/date meaning is not inferred from object-name strings.
func InvalidDetailsJSON(ctx context.Context, q rowset.Queryer, major int, owners []string, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return InvalidDetailsRows(ctx, q, major, owners...)
	}, invalidDetailsSchema(), limits, rowset.EnvelopeOptions{
		Provenance: invalidDetailsProvenance(major, owners),
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := DecodeInvalidDetails(result, major, owners...)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "encode" {
			return &ReadError{cause: cause}
		}
		return cause
	})
}

// InvalidDetailsOptions carries the source selectors for the options-based fact APIs.
type InvalidDetailsOptions struct {
	OracleMajor int
	Owners      []string
}

// InvalidDetailsStatementWithOptions selects the same source as InvalidDetailsStatement using one options value.
func InvalidDetailsStatementWithOptions(options InvalidDetailsOptions) (string, []any, error) {
	return InvalidDetailsStatement(options.OracleMajor, options.Owners...)
}

// DecodeInvalidDetailsWithOptions selects the same source as DecodeInvalidDetails using one options value.
func DecodeInvalidDetailsWithOptions(result session.Result, options InvalidDetailsOptions) (InvalidDetailsResult, error) {
	return DecodeInvalidDetails(result, options.OracleMajor, options.Owners...)
}

// InvalidDetailsRowsWithOptions selects the same source as InvalidDetailsRows using one options value.
func InvalidDetailsRowsWithOptions(ctx context.Context, q rowset.Queryer, options InvalidDetailsOptions) (*sql.Rows, error) {
	return InvalidDetailsRows(ctx, q, options.OracleMajor, options.Owners...)
}

// InvalidDetailsJSONWithOptions selects the same source as InvalidDetailsJSON using one options value.
func InvalidDetailsJSONWithOptions(ctx context.Context, q rowset.Queryer, options InvalidDetailsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return InvalidDetailsJSON(ctx, q, options.OracleMajor, options.Owners, limits)
}
