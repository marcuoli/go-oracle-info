package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MaxObjectStatusKeys is the largest admitted exact-key batch.
const MaxObjectStatusKeys = 100

// ObjectStatusKey binds one exact owner, type and name. A nil SubobjectName
// leaves subobject identity unobserved and selects all matching candidates.
// A nonnil value requires the exact subobject name or SQL NULL.
type ObjectStatusKey struct {
	Owner         string         `json:"owner"`
	ObjectType    string         `json:"object_type"`
	ObjectName    string         `json:"object_name"`
	SubobjectName *session.Value `json:"subobject_name,omitempty"`
}

// ObjectStatusOptions admits a numeric release and one nonempty bounded batch.
type ObjectStatusOptions struct {
	OracleRelease string
	Keys          []ObjectStatusKey
}

// ObjectStatusRequest supplies the caller-owned reference and finite read limits.
type ObjectStatusRequest struct {
	Options   ObjectStatusOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// ObjectStatusProvenance describes which keys were selected in the connected
// dictionary. Complete describes transport completion, not coverage across editions.
type ObjectStatusProvenance struct {
	Source        string            `json:"source"`
	OracleRelease string            `json:"oracle_release"`
	Scope         string            `json:"scope"`
	Keys          []ObjectStatusKey `json:"keys"`
	Complete      bool              `json:"complete"`
}

// ObjectStatusRow retains an individual dictionary candidate and its raw status.
// A nil EditionName means the release projection omitted that column.
type ObjectStatusRow struct {
	Owner, ObjectType, ObjectName, SubobjectName, Status session.Value
	EditionName                                          *session.Value
}

// ObjectStatusResult retains the selected rows in source order, including
// duplicates. Missing keys have no rows and imply no status.
type ObjectStatusResult struct {
	Provenance ObjectStatusProvenance
	Rows       []ObjectStatusRow
}

type objectStatusSelection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance ObjectStatusProvenance
}

const objectStatusSelect = `
SELECT o.OWNER AS OWNER, o.OBJECT_TYPE AS OBJECT_TYPE, o.OBJECT_NAME AS OBJECT_NAME,
       o.SUBOBJECT_NAME AS SUBOBJECT_NAME, o.STATUS AS STATUS`

const objectStatusMatch = `
FROM DBA_OBJECTS o
WHERE EXISTS (
    SELECT 1 FROM requested_objects k
    WHERE o.OWNER = k.OWNER_NAME
      AND o.OBJECT_TYPE = k.OBJECT_TYPE
      AND o.OBJECT_NAME = k.OBJECT_NAME
      AND NLSSORT(o.OWNER, 'NLS_SORT=BINARY') = NLSSORT(k.OWNER_NAME, 'NLS_SORT=BINARY')
      AND NLSSORT(o.OBJECT_TYPE, 'NLS_SORT=BINARY') = NLSSORT(k.OBJECT_TYPE, 'NLS_SORT=BINARY')
      AND NLSSORT(o.OBJECT_NAME, 'NLS_SORT=BINARY') = NLSSORT(k.OBJECT_NAME, 'NLS_SORT=BINARY')
      AND (k.SUBOBJECT_MODE = 0
           OR (k.SUBOBJECT_MODE = 1 AND o.SUBOBJECT_NAME IS NULL)
           OR (k.SUBOBJECT_MODE = 2 AND o.SUBOBJECT_NAME = k.SUBOBJECT_NAME
               AND NLSSORT(o.SUBOBJECT_NAME, 'NLS_SORT=BINARY') = NLSSORT(k.SUBOBJECT_NAME, 'NLS_SORT=BINARY')))
)
ORDER BY o.OWNER, o.OBJECT_TYPE, o.OBJECT_NAME, o.SUBOBJECT_NAME, o.STATUS`

func validObjectStatusName(name string) bool {
	return name != "" && len(name) <= 128 && utf8.ValidString(name) && !strings.ContainsRune(name, 0)
}

func selectObjectStatus(options ObjectStatusOptions) (objectStatusSelection, error) {
	version, ok := oracleversion.Parse(options.OracleRelease)
	if !ok || len(options.Keys) == 0 || len(options.Keys) > MaxObjectStatusKeys {
		return objectStatusSelection{}, ErrInvalidRequest
	}
	major, supported := oracleversion.LegacyFactMajor(options.OracleRelease)
	if !supported || major < 11 {
		return objectStatusSelection{}, ErrUnsupportedVersion
	}
	// Oracle 11.1 reserves EDITION_NAME. From 11.2 it identifies the actual
	// object's edition in the caller's current-edition dictionary projection.
	edition := major > 11 || version[1] == 2
	schema := []rowset.Column{
		{Name: "OWNER", Kind: rowset.Text},
		{Name: "OBJECT_TYPE", Kind: rowset.Text},
		{Name: "OBJECT_NAME", Kind: rowset.Text},
		{Name: "SUBOBJECT_NAME", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
	}
	keys := make([]ObjectStatusKey, len(options.Keys))
	args := make([]any, 0, len(keys)*4)
	var query strings.Builder
	query.WriteString("WITH requested_objects (OWNER_NAME, OBJECT_TYPE, OBJECT_NAME, SUBOBJECT_MODE, SUBOBJECT_NAME) AS (")
	for i, key := range options.Keys {
		if !validObjectStatusName(key.Owner) || !validObjectStatusName(key.ObjectType) || !validObjectStatusName(key.ObjectName) {
			return objectStatusSelection{}, ErrInvalidRequest
		}
		keys[i] = key
		if i > 0 {
			query.WriteString(" UNION ALL ")
		}
		first := len(args) + 1
		args = append(args, key.Owner, key.ObjectType, key.ObjectName)
		fmt.Fprintf(&query, "SELECT CAST(:%d AS VARCHAR2(128 CHAR)), CAST(:%d AS VARCHAR2(128 CHAR)), CAST(:%d AS VARCHAR2(128 CHAR)), ", first, first+1, first+2)
		switch {
		case key.SubobjectName == nil:
			query.WriteString("0, CAST(NULL AS VARCHAR2(128 CHAR)) FROM DUAL")
		case key.SubobjectName.Null && key.SubobjectName.Text == "":
			keys[i].SubobjectName = new(*key.SubobjectName)
			query.WriteString("1, CAST(NULL AS VARCHAR2(128 CHAR)) FROM DUAL")
		case !key.SubobjectName.Null && validObjectStatusName(key.SubobjectName.Text):
			keys[i].SubobjectName = new(*key.SubobjectName)
			args = append(args, key.SubobjectName.Text)
			fmt.Fprintf(&query, "2, CAST(:%d AS VARCHAR2(128 CHAR)) FROM DUAL", len(args))
		default:
			return objectStatusSelection{}, ErrInvalidRequest
		}
	}
	query.WriteString(")" + objectStatusSelect)
	if edition {
		query.WriteString(", o.EDITION_NAME AS EDITION_NAME")
		schema = append(schema, rowset.Column{Name: "EDITION_NAME", Kind: rowset.Text})
	}
	// The semijoin confines selection to requested keys and never multiplies
	// dictionary rows when supplied keys overlap. Binary checks keep case exact
	// under linguistic session settings.
	query.WriteString(objectStatusMatch)
	if edition {
		query.WriteString(", o.EDITION_NAME")
	}
	return objectStatusSelection{
		sql: query.String(), args: args, schema: schema,
		provenance: ObjectStatusProvenance{Source: "DBA_OBJECTS", OracleRelease: options.OracleRelease, Scope: "current_container_visible_exact_keys", Keys: keys},
	}, nil
}

// ObjectStatusStatement builds one bounded, bind-only dictionary selection.
func ObjectStatusStatement(options ObjectStatusOptions) (string, []any, error) {
	plan, err := selectObjectStatus(options)
	return plan.sql, plan.args, err
}

// DecodeObjectStatus validates all returned candidates against the request.
// A pure decoder cannot attest cursor or transport completion.
func DecodeObjectStatus(result session.Result, options ObjectStatusOptions) (ObjectStatusResult, error) {
	plan, err := selectObjectStatus(options)
	if err != nil {
		return ObjectStatusResult{}, err
	}
	return decodeObjectStatus(result, plan)
}

func decodeObjectStatus(result session.Result, plan objectStatusSelection) (ObjectStatusResult, error) {
	index, err := factColumns(result, plan.schema)
	if err != nil {
		return ObjectStatusResult{}, err
	}
	rows := make([]ObjectStatusRow, len(result.Rows))
	for i, row := range result.Rows {
		fact := ObjectStatusRow{
			Owner: row[index["OWNER"]], ObjectType: row[index["OBJECT_TYPE"]],
			ObjectName: row[index["OBJECT_NAME"]], SubobjectName: row[index["SUBOBJECT_NAME"]],
			Status: row[index["STATUS"]],
		}
		if editionIndex, selected := index["EDITION_NAME"]; selected {
			fact.EditionName = new(row[editionIndex])
		}
		if fact.Owner.Null || fact.ObjectType.Null || fact.ObjectName.Null || !matchesObjectStatusKey(fact, plan.provenance.Keys) {
			return ObjectStatusResult{}, ErrInvalidEvidence
		}
		rows[i] = fact
	}
	return ObjectStatusResult{Provenance: plan.provenance, Rows: rows}, nil
}

func matchesObjectStatusKey(row ObjectStatusRow, keys []ObjectStatusKey) bool {
	for _, key := range keys {
		if row.Owner.Text != key.Owner || row.ObjectType.Text != key.ObjectType || row.ObjectName.Text != key.ObjectName {
			continue
		}
		if key.SubobjectName == nil || *key.SubobjectName == row.SubobjectName {
			return true
		}
	}
	return false
}

// ReadObjectStatus reads one batch on a caller-admitted reader and returns
// no successful prefix after query, cancellation, bounds or evidence failure.
func ReadObjectStatus(ctx context.Context, reader Reader, request ObjectStatusRequest) (ObjectStatusResult, error) {
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ObjectStatusResult{}, ErrInvalidRequest
	}
	plan, err := selectObjectStatus(request.Options)
	if err != nil {
		return ObjectStatusResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ObjectStatusResult{}, err
	}
	if err := factReadError(ctx, nil); err != nil {
		return ObjectStatusResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql, Args: plan.args})
	if err := factReadError(ctx, err); err != nil {
		return ObjectStatusResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return ObjectStatusResult{}, &ReadError{cause: rowset.ErrCapacity}
	}
	facts, err := decodeObjectStatus(result, plan)
	if err := factReadError(ctx, err); err != nil {
		return ObjectStatusResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ObjectStatusRows returns the actual native cursor. The caller owns iteration,
// finite budgets, terminal errors and close.
func ObjectStatusRows(ctx context.Context, q rowset.Queryer, options ObjectStatusOptions) (*sql.Rows, error) {
	plan, err := selectObjectStatus(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// ObjectStatusJSON independently consumes and validates a complete bounded cursor.
func ObjectStatusJSON(ctx context.Context, q rowset.Queryer, options ObjectStatusOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := selectObjectStatus(options)
	if err != nil {
		return nil, err
	}
	plan.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql, plan.args...), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := decodeObjectStatus(result, plan)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &ReadError{cause: cause}
	}, "limits", "header")
}
