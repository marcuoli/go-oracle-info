package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ErrUnsupportedLayout identifies a visible object without a reviewed heartbeat
// structure. It is distinct from no visible object, unsupported release or denial.
var ErrUnsupportedLayout = errors.New("unsupported GoldenGate heartbeat layout")

// HeartbeatCatalogOptions selects accessible fixed-name objects in the admitted
// container. Schema is optional, exact and never inferred from grants.
type HeartbeatCatalogOptions struct{ OracleVersion, Schema string }

// HeartbeatCatalogRequest retains the caller's admitted SQL reference.
type HeartbeatCatalogRequest struct {
	Options   HeartbeatCatalogOptions
	Reference reference.Reference
}

// HeartbeatColumn preserves raw accessible dictionary column/type evidence.
type HeartbeatColumn struct{ Name, DataType, DataTypeOwner session.Value }

// HeartbeatSource is a structurally verified object obtained from a decoded
// catalog. It contains no handle/lease, cache or claim of continuing accessibility.
// Reuse requires caller-proven endpoint/container affinity; DDL or grants can race
// subsequent reads, whose errors remain errors. A zero source is not admitted.
type HeartbeatSource struct{ owner, name, version string }

// Owner returns the exact catalog owner.
func (s HeartbeatSource) Owner() string { return s.owner }

// Name returns the verified fixed object name.
func (s HeartbeatSource) Name() string { return s.name }

// HeartbeatObject preserves each visible candidate, including rejected layouts.
// Supported is structural compatibility, never Oracle origin or GGSCHEMA proof.
type HeartbeatObject struct {
	Owner, Name, ObjectType, Status session.Value
	Columns                         []HeartbeatColumn
	Supported                       bool
	Reason                          string
	source                          HeartbeatSource
}

// Source admits only a verified fixed-name TABLE/VIEW layout. Rejected candidates
// remain in the catalog; they never trigger SELECTs against guessed schemas.
func (o HeartbeatObject) Source() (HeartbeatSource, error) {
	if !o.Supported || o.source.owner == "" {
		return HeartbeatSource{}, ErrUnsupportedLayout
	}
	return o.source, nil
}

// HeartbeatCatalogResult preserves accessible candidates and full column metadata.
// Empty does not distinguish absent objects from objects invisible to this login.
type HeartbeatCatalogResult struct {
	Provenance Provenance
	Rows       []HeartbeatObject
}

func heartbeatCatalogPlan(o HeartbeatCatalogOptions) (selection, error) {
	if !discoveryVersion(o.OracleVersion) {
		return selection{}, ErrUnsupportedVersion
	}
	if err := ValidateDiscoveryFilter(DiscoveryFilter{Schema: o.Schema}); err != nil {
		return selection{}, err
	}
	p := selection{sql: "SELECT v.OWNER AS OWNER, v.OBJECT_NAME AS OBJECT_NAME, v.OBJECT_TYPE AS OBJECT_TYPE, v.STATUS AS STATUS, c.COLUMN_NAME AS COLUMN_NAME, c.DATA_TYPE AS DATA_TYPE, c.DATA_TYPE_OWNER AS DATA_TYPE_OWNER FROM ALL_OBJECTS v LEFT JOIN ALL_TAB_COLUMNS c ON NLSSORT(c.OWNER, 'NLS_SORT=BINARY') = NLSSORT(v.OWNER, 'NLS_SORT=BINARY') AND NLSSORT(c.TABLE_NAME, 'NLS_SORT=BINARY') = NLSSORT(v.OBJECT_NAME, 'NLS_SORT=BINARY') AND v.OBJECT_TYPE IN ('TABLE', 'VIEW') WHERE v.SUBOBJECT_NAME IS NULL AND NLSSORT(v.OBJECT_NAME, 'NLS_SORT=BINARY') IN (NLSSORT('GG_HEARTBEAT', 'NLS_SORT=BINARY'), NLSSORT('GG_LAG', 'NLS_SORT=BINARY'))", provenance: Provenance{Source: "ALL_OBJECTS/ALL_TAB_COLUMNS", Scope: "accessible_objects_current_container", OracleVersion: o.OracleVersion}, required: []string{"OWNER", "OBJECT_NAME", "OBJECT_TYPE"}, catalog: true}
	for _, name := range []string{"OWNER", "OBJECT_NAME", "OBJECT_TYPE", "STATUS", "COLUMN_NAME", "DATA_TYPE", "DATA_TYPE_OWNER"} {
		p.schema = append(p.schema, rowset.Column{Name: name, Kind: rowset.Text})
	}
	if o.Schema != "" {
		addDiscoveryFilter(&p, []string{"OWNER"}, []string{o.Schema})
	}
	p.sql += " ORDER BY v.OWNER, v.OBJECT_NAME, v.OBJECT_TYPE, c.COLUMN_ID, c.COLUMN_NAME"
	return p, nil
}

// HeartbeatCatalogStatement selects only accessible GG_HEARTBEAT/GG_LAG names,
// retaining wrong object types, invalid objects and missing/incompatible columns.
func HeartbeatCatalogStatement(o HeartbeatCatalogOptions) (string, []any, error) {
	p, e := heartbeatCatalogPlan(o)
	return p.sql, p.args, e
}

// DecodeHeartbeatCatalog groups complete supplied metadata by owner/name/type.
// Extra columns are retained and allowed. Missing, duplicate, user-defined or
// incompatible required types reject the candidate without hiding other owners.
func DecodeHeartbeatCatalog(r session.Result, o HeartbeatCatalogOptions) (HeartbeatCatalogResult, error) {
	p, err := heartbeatCatalogPlan(o)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	idx, err := columns(r, p)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	objects := make([]HeartbeatObject, 0)
	positions := map[[3]string]int{}
	for _, row := range r.Rows {
		owner, name, kind := row[idx["OWNER"]], row[idx["OBJECT_NAME"]], row[idx["OBJECT_TYPE"]]
		key := [3]string{owner.Text, name.Text, kind.Text}
		pos, ok := positions[key]
		if !ok {
			pos = len(objects)
			positions[key] = pos
			objects = append(objects, HeartbeatObject{Owner: owner, Name: name, ObjectType: kind, Status: row[idx["STATUS"]], Columns: make([]HeartbeatColumn, 0)})
		}
		object := &objects[pos]
		if object.Status != row[idx["STATUS"]] {
			return HeartbeatCatalogResult{}, ErrInvalidEvidence
		}
		column := HeartbeatColumn{Name: row[idx["COLUMN_NAME"]], DataType: row[idx["DATA_TYPE"]], DataTypeOwner: row[idx["DATA_TYPE_OWNER"]]}
		if !column.Name.Null {
			object.Columns = append(object.Columns, column)
		}
	}
	for i := range objects {
		object := &objects[i]
		expectedType := "TABLE"
		if object.Name.Text == "GG_LAG" {
			expectedType = "VIEW"
		}
		object.Reason = "missing_or_incompatible_columns"
		if object.ObjectType.Text != expectedType {
			object.Reason = "unsupported_object_type"
			continue
		}
		if object.Status.Null || object.Status.Text != "VALID" {
			object.Reason = "object_not_valid"
			continue
		}
		fields := heartbeatFields(object.Name.Text)
		observed := map[string]HeartbeatColumn{}
		duplicate := false
		for _, column := range object.Columns {
			if _, ok := observed[column.Name.Text]; ok {
				duplicate = true
			}
			observed[column.Name.Text] = column
		}
		compatible := !duplicate
		for _, field := range fields {
			c, ok := observed[field.name]
			compatible = compatible && ok && !c.DataType.Null && c.DataType.Text == field.dataType && c.DataTypeOwner.Null
		}
		if compatible {
			object.Supported = true
			object.Reason = ""
			object.source = HeartbeatSource{owner: object.Owner.Text, name: object.Name.Text, version: o.OracleVersion}
		}
	}
	return HeartbeatCatalogResult{Provenance: p.provenance, Rows: objects}, nil
}

// ReadHeartbeatCatalog executes one bounded dictionary read without schema guesses.
func ReadHeartbeatCatalog(ctx context.Context, r Reader, req HeartbeatCatalogRequest) (HeartbeatCatalogResult, error) {
	p, err := heartbeatCatalogPlan(req.Options)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	result, err := query(ctx, r, Request{OracleVersion: req.Options.OracleVersion, Reference: req.Reference}, p)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	facts, err := DecodeHeartbeatCatalog(result, req.Options)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return HeartbeatCatalogResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// HeartbeatCatalogRows returns the actual native metadata cursor. Callers own
// Next/Scan/Err/Close, handle affinity, deadlines and finite streaming budgets.
func HeartbeatCatalogRows(ctx context.Context, q rowset.Queryer, o HeartbeatCatalogOptions) (*sql.Rows, error) {
	p, err := heartbeatCatalogPlan(o)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// HeartbeatCatalogJSON returns complete raw metadata with provenance. Unsupported
// layouts remain metadata, not a failed/truncated catalog or guessed data query.
func HeartbeatCatalogJSON(ctx context.Context, q rowset.Queryer, o HeartbeatCatalogOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := heartbeatCatalogPlan(o)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}

// ReadNativeHeartbeatCatalog borrows the caller's native handle and delegates all
// bounded consumption/finalization to shared rowset, retaining the caller ref.
func ReadNativeHeartbeatCatalog(ctx context.Context, q rowset.Queryer, req HeartbeatCatalogRequest, limits rowset.Limits) (HeartbeatCatalogResult, error) {
	p, err := heartbeatCatalogPlan(req.Options)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return HeartbeatCatalogResult{}, err
	}
	return ReadHeartbeatCatalog(ctx, nativeFactReader{q: q, plan: p, limits: resolved}, req)
}

func catalogColumns(row []session.Value, idx map[string]int) bool {
	if !identifier(row[idx["OWNER"]].Text) {
		return false
	}
	name := row[idx["OBJECT_NAME"]].Text
	if name != "GG_HEARTBEAT" && name != "GG_LAG" {
		return false
	}
	column, typ, owner := row[idx["COLUMN_NAME"]], row[idx["DATA_TYPE"]], row[idx["DATA_TYPE_OWNER"]]
	if column.Null {
		return typ.Null && owner.Null
	}
	return identifier(column.Text) && !typ.Null && strings.TrimSpace(typ.Text) != ""
}
