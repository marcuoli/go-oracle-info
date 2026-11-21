package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Instance retains source instance identity. StartupLocal has no inferred timezone.
type Instance struct {
	ID           int
	Name         Text
	HostName     Text
	Version      Text
	StartupLocal Text
}

// InstanceStatement returns current-instance identity and a timezone-free startup
// label. It does not describe all RAC members or establish container admission.
func InstanceStatement() string {
	return "SELECT TO_CHAR(INSTANCE_NUMBER,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS INSTANCE_ID, INSTANCE_NAME AS INSTANCE_NAME, HOST_NAME AS HOST_NAME, VERSION AS VERSION, TO_CHAR(STARTUP_TIME,'YYYY-MM-DD\"T\"HH24:MI:SS') AS STARTUP_LOCAL FROM V$INSTANCE"
}

// DecodeInstance rejects an invalid instance ID rather than inventing identity.
func DecodeInstance(result session.Result) (Instance, error) {
	r, absent, err := row(result, "INSTANCE_ID", "INSTANCE_NAME", "HOST_NAME", "VERSION", "STARTUP_LOCAL")
	if err != nil {
		return Instance{}, err
	}
	if absent {
		return Instance{Name: Text{State: NoRows}, HostName: Text{State: NoRows}, Version: Text{State: NoRows}, StartupLocal: Text{State: NoRows}}, nil
	}
	id, err := strconv.Atoi(r["INSTANCE_ID"].Text)
	if err != nil || r["INSTANCE_ID"].Null || id <= 0 {
		return Instance{}, ErrInvalidEvidence
	}
	return Instance{ID: id, Name: text(r["INSTANCE_NAME"]), HostName: text(r["HOST_NAME"]), Version: text(r["VERSION"]), StartupLocal: text(r["STARTUP_LOCAL"])}, nil
}

// InstanceIdentityRow preserves source values and SQL NULL without application policy.
type InstanceIdentityRow struct {
	ID           session.Value
	Name         session.Value
	Host         session.Value
	Version      session.Value
	StartupLocal session.Value
}

// InstanceIdentityOptions admits the independently reviewed source selection.
// New projections admit numeric majors 19, 21 and 23. Oracle 26ai uses its
// observed numeric version. Older compatibility selectors remain unchanged.
type InstanceIdentityOptions struct{ OracleMajor int }

// InstanceIdentityRequest supplies admitted options and a caller-owned SQL reference.
type InstanceIdentityRequest struct {
	Options   InstanceIdentityOptions
	Reference reference.Reference
}

// InstanceIdentityResult retains source order and duplicates. Pure Decode does not
// establish transport completion; successful owned Read/JSON does.
type InstanceIdentityResult struct {
	Provenance DatasetProvenance
	Rows       []InstanceIdentityRow
}

// InstanceIdentityStatement selects reviewed SQL with bound values, owning no connection.
func InstanceIdentityStatement(options InstanceIdentityOptions) (string, []any, error) {
	p, err := instanceIdentityDatasetPlan(options)
	return p.sql, p.args, err
}

func instanceIdentityDatasetPlan(options InstanceIdentityOptions) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	query := InstanceStatement()
	return datasetSelection{sql: query, args: nil, schema: []rowset.Column{{Name: "INSTANCE_ID", Kind: rowset.Text}, {Name: "INSTANCE_NAME", Kind: rowset.Text}, {Name: "HOST_NAME", Kind: rowset.Text}, {Name: "VERSION", Kind: rowset.Text}, {Name: "STARTUP_LOCAL", Kind: rowset.Text}}, provenance: DatasetProvenance{Source: "V$INSTANCE", OracleMajor: options.OracleMajor, Scope: "current_instance_identity_startup_local_label"}}, nil
}

// DecodeInstanceIdentity validates the entire shape and every row before returning facts.
func DecodeInstanceIdentity(result session.Result, options InstanceIdentityOptions) (InstanceIdentityResult, error) {
	p, err := instanceIdentityDatasetPlan(options)
	if err != nil {
		return InstanceIdentityResult{}, err
	}
	return decodeDatasetInstanceIdentity(result, p)
}

func decodeDatasetInstanceIdentity(result session.Result, p datasetSelection) (InstanceIdentityResult, error) {
	index, err := datasetColumns(result, p, "INSTANCE_ID")
	if err != nil {
		return InstanceIdentityResult{}, err
	}
	if len(result.Rows) > 1 {
		return InstanceIdentityResult{}, ErrInvalidEvidence
	}
	rows := make([]InstanceIdentityRow, len(result.Rows))
	for i, r := range result.Rows {
		rows[i] = InstanceIdentityRow{ID: r[index["INSTANCE_ID"]], Name: r[index["INSTANCE_NAME"]], Host: r[index["HOST_NAME"]], Version: r[index["VERSION"]], StartupLocal: r[index["STARTUP_LOCAL"]]}
	}
	return InstanceIdentityResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadInstanceIdentity executes once through an admitted complete bounded reader.
// Query/terminal/close/cancellation/domain failures publish no successful prefix.
func ReadInstanceIdentity(ctx context.Context, reader Reader, request InstanceIdentityRequest) (InstanceIdentityResult, error) {
	p, err := instanceIdentityDatasetPlan(request.Options)
	if err != nil {
		return InstanceIdentityResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return InstanceIdentityResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return InstanceIdentityResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return InstanceIdentityResult{}, err
	}
	facts, err := decodeDatasetInstanceIdentity(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return InstanceIdentityResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// InstanceIdentityRows returns the actual caller-native cursor; its caller owns
// Next/Scan/Err/Close, affinity, cancellation and finite stream budgets.
func InstanceIdentityRows(ctx context.Context, q rowset.Queryer, options InstanceIdentityOptions) (*sql.Rows, error) {
	p, err := instanceIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	r, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &DatasetError{p.provenance.Source, err}
	}
	return r, nil
}

// InstanceIdentityJSON consumes an independent complete cursor through shared rowset,
// validates domain facts and publishes nothing on any failure or budget overflow.
func InstanceIdentityJSON(ctx context.Context, q rowset.Queryer, options InstanceIdentityOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := instanceIdentityDatasetPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeDatasetInstanceIdentity(r, p); return err })
}
