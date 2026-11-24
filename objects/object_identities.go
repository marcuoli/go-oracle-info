package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ObjectIdentitiesOptions identifies the numeric release and exact admitted owners.
type ObjectIdentitiesOptions struct {
	OracleVersion string
	Owners        []string
}

// ObjectIdentitiesRequest carries caller admission, reference, and buffered limits.
type ObjectIdentitiesRequest struct {
	Options   ObjectIdentitiesOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// ObjectIdentity is one visible dictionary row. EditionName and SourceContainerID
// are nil on releases that do not project them.
type ObjectIdentity struct {
	Owner, Name, SubobjectName, ObjectType, ObjectID session.Value
	EditionName, SourceContainerID                   *session.Value
}

// ObjectIdentitiesResult retains selected rows and source provenance.
type ObjectIdentitiesResult struct {
	Provenance DatasetProvenance
	Rows       []ObjectIdentity
}

// ObjectIdentitiesStatement selects the reviewed projection without truncation.
func ObjectIdentitiesStatement(options ObjectIdentitiesOptions) (string, []any, error) {
	p, err := objectIdentitiesPlan(options)
	return p.sql, p.args, err
}

func objectIdentitiesPlan(options ObjectIdentitiesOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	if len(options.Owners) == 0 || len(options.Owners) > 1000 {
		return datasetSelection{}, ErrInvalidRequest
	}
	binds := make([]string, len(options.Owners))
	args := make([]any, len(options.Owners))
	for i, name := range options.Owners {
		if !validObjectStatusName(name) {
			return datasetSelection{}, ErrInvalidRequest
		}
		binds[i] = "NLSSORT(:" + strconv.Itoa(i+1) + ", 'NLS_SORT=BINARY')"
		args[i] = name
	}
	p := datasetSelection{
		sql: "SELECT v.OWNER AS OWNER, v.OBJECT_NAME AS OBJECT_NAME, v.SUBOBJECT_NAME AS SUBOBJECT_NAME, " +
			"v.OBJECT_TYPE AS OBJECT_TYPE, TO_CHAR(v.OBJECT_ID, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS OBJECT_ID",
		args: args,
		schema: []rowset.Column{
			{Name: "OWNER", Kind: rowset.Text},
			{Name: "OBJECT_NAME", Kind: rowset.Text},
			{Name: "SUBOBJECT_NAME", Kind: rowset.Text},
			{Name: "OBJECT_TYPE", Kind: rowset.Text},
			{Name: "OBJECT_ID", Kind: rowset.Text},
		},
		provenance: DatasetProvenance{Source: "DBA_OBJECTS", OracleMajor: major, Scope: "current_edition_dictionary", Owners: append([]string(nil), options.Owners...)},
	}
	version, _ := oracleversion.Parse(options.OracleVersion)
	// The 11.1 reference reserves this field. It identifies an edition starting in 11.2.
	if major > 11 || major == 11 && version[1] == 2 {
		p.sql += ", v.EDITION_NAME AS EDITION_NAME"
		p.schema = append(p.schema, rowset.Column{Name: "EDITION_NAME", Kind: rowset.Text})
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DBA_OBJECTS v WHERE NLSSORT(v.OWNER, 'NLS_SORT=BINARY') IN (" + strings.Join(binds, ", ") + ") ORDER BY v.OBJECT_ID"
	return p, nil
}

// DecodeObjectIdentities validates the complete supplied projection before returning any rows.
func DecodeObjectIdentities(result session.Result, options ObjectIdentitiesOptions) (ObjectIdentitiesResult, error) {
	p, err := objectIdentitiesPlan(options)
	if err != nil {
		return ObjectIdentitiesResult{}, err
	}
	return decodeObjectIdentities(result, p)
}

func decodeObjectIdentities(result session.Result, p datasetSelection) (ObjectIdentitiesResult, error) {
	index, err := datasetColumns(result, p, "OBJECT_ID")
	if err != nil {
		return ObjectIdentitiesResult{}, err
	}
	owners := make(map[string]bool, len(p.provenance.Owners))
	for _, owner := range p.provenance.Owners {
		owners[owner] = true
	}
	rows := make([]ObjectIdentity, len(result.Rows))
	for i, row := range result.Rows {
		item := ObjectIdentity{
			Owner: row[index["OWNER"]], Name: row[index["OBJECT_NAME"]],
			SubobjectName: row[index["SUBOBJECT_NAME"]], ObjectType: row[index["OBJECT_TYPE"]], ObjectID: row[index["OBJECT_ID"]],
		}
		if !owners[item.Owner.Text] || item.Name.Text == "" {
			return ObjectIdentitiesResult{}, ErrInvalidEvidence
		}
		if j, ok := index["EDITION_NAME"]; ok {
			v := row[j]
			item.EditionName = &v
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			v := row[j]
			if v.Null || !numeric.IsUnsignedInteger(v.Text) {
				return ObjectIdentitiesResult{}, ErrInvalidEvidence
			}
			item.SourceContainerID = &v
		}
		rows[i] = item
	}
	return ObjectIdentitiesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadObjectIdentities executes once through the caller's admitted bounded reader.
func ReadObjectIdentities(ctx context.Context, reader Reader, request ObjectIdentitiesRequest) (ObjectIdentitiesResult, error) {
	p, err := objectIdentitiesPlan(request.Options)
	if err != nil {
		return ObjectIdentitiesResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return ObjectIdentitiesResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ObjectIdentitiesResult{}, err
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return ObjectIdentitiesResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql, Args: p.args})
	if err = datasetReadError(ctx, p, err); err != nil {
		return ObjectIdentitiesResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return ObjectIdentitiesResult{}, datasetReadError(ctx, p, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeObjectIdentities(result, p)
	if err = datasetReadError(ctx, p, decodeErr); err != nil {
		return ObjectIdentitiesResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ObjectIdentitiesRows transfers the caller-native cursor.
func ObjectIdentitiesRows(ctx context.Context, q rowset.Queryer, options ObjectIdentitiesOptions) (*sql.Rows, error) {
	p, err := objectIdentitiesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return rows, nil
}

// ObjectIdentitiesJSON consumes and validates a separate complete bounded cursor.
func ObjectIdentitiesJSON(ctx context.Context, q rowset.Queryer, options ObjectIdentitiesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := objectIdentitiesPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeObjectIdentities(result, p)
		return err
	})
}
