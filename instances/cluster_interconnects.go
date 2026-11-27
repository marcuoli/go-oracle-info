package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ClusterInterconnectsOptions carries the observed numeric Oracle release.
type ClusterInterconnectsOptions struct{ OracleVersion string }

// ClusterInterconnectsRequest adds the caller's SQL reference and buffered limits.
type ClusterInterconnectsRequest struct {
	Options   ClusterInterconnectsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// ClusterInterconnectRow is one runtime interface observation by a reporting
// instance. ContainerID is absent before 12c and can be SQL NULL when projected.
// IPAddress, IsPublic and Source retain the database's raw nullable values.
type ClusterInterconnectRow struct {
	InstanceID                        session.Value
	ContainerID                       *session.Value
	Name, IPAddress, IsPublic, Source session.Value
}

// ClusterInterconnectsResult retains the returned order and duplicates. Complete
// describes the read. It does not establish the set of configured nodes,
// listeners, or SCAN or node VIPs.
type ClusterInterconnectsResult struct {
	State      State
	Provenance VisibilityProvenance
	Rows       []ClusterInterconnectRow
}

// ClusterInterconnectsStatement selects the common runtime interconnect columns.
func ClusterInterconnectsStatement(options ClusterInterconnectsOptions) (string, error) {
	plan, err := clusterInterconnectsPlan(options)
	return plan.sql, err
}

func clusterInterconnectsPlan(options ClusterInterconnectsOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	version, _ := oracleversion.Parse(options.OracleVersion)
	if !ok || major < 10 || (major == 10 && version[1] < 2) {
		return selection{}, ErrUnsupportedVersion
	}
	plan := selection{
		sql: "SELECT i.INST_ID AS INST_ID, i.NAME AS NAME, i.IP_ADDRESS AS IP_ADDRESS, i.IS_PUBLIC AS IS_PUBLIC, i.SOURCE AS SOURCE",
		schema: []rowset.Column{
			{Name: "INST_ID", Kind: rowset.Integer},
			{Name: "NAME", Kind: rowset.Text},
			{Name: "IP_ADDRESS", Kind: rowset.Text},
			{Name: "IS_PUBLIC", Kind: rowset.Text},
			{Name: "SOURCE", Kind: rowset.Text},
		},
		provenance: Provenance{Source: "GV$CLUSTER_INTERCONNECTS", OracleMajor: major, Scope: "visible_runtime_interconnects"},
	}
	if major >= 12 {
		plan.sql += ", i.CON_ID AS CON_ID"
		plan.schema = append(plan.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	// Use a simple fixed-view scan so grouping or ordering does not require
	// additional RAC query work. The caller can order its observations.
	plan.sql += " FROM SYS.GV_$CLUSTER_INTERCONNECTS i"
	return plan, nil
}

// DecodeClusterInterconnects validates all supplied rows without asserting transport completion.
func DecodeClusterInterconnects(result session.Result, options ClusterInterconnectsOptions) (ClusterInterconnectsResult, error) {
	plan, err := clusterInterconnectsPlan(options)
	if err != nil {
		return ClusterInterconnectsResult{State: Unsupported}, err
	}
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion}
	return decodeClusterInterconnects(result, plan, p)
}

func decodeClusterInterconnects(result session.Result, plan selection, p VisibilityProvenance) (ClusterInterconnectsResult, error) {
	facts := ClusterInterconnectsResult{State: Failed, Provenance: p}
	index, err := columns(result, plan.schema)
	if err != nil {
		return facts, &Error{Operation: "decode", cause: err}
	}
	rows := make([]ClusterInterconnectRow, len(result.Rows))
	for i, row := range result.Rows {
		item := ClusterInterconnectRow{
			InstanceID: row[index["INST_ID"]],
			Name:       row[index["NAME"]],
			IPAddress:  row[index["IP_ADDRESS"]],
			IsPublic:   row[index["IS_PUBLIC"]],
			Source:     row[index["SOURCE"]],
		}
		if pos, ok := index["CON_ID"]; ok {
			value := row[pos]
			item.ContainerID = &value
		}
		rows[i] = item
	}
	facts.State = NoRows
	if len(rows) != 0 {
		facts.State = Available
	}
	facts.Rows = rows
	return facts, nil
}

// ReadClusterInterconnects reads once from the admitted reader within its result
// bounds. A failed read returns no partial rows and retains the original cause.
func ReadClusterInterconnects(ctx context.Context, reader Reader, request ClusterInterconnectsRequest) (ClusterInterconnectsResult, error) {
	plan, err := clusterInterconnectsPlan(request.Options)
	if err != nil {
		return ClusterInterconnectsResult{State: Unsupported}, err
	}
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: request.Options.OracleVersion}
	failure := ClusterInterconnectsResult{State: Failed, Provenance: p}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return failure, err
	}
	// An owned reader can return an error from iteration or close. Only the
	// native adapters below can identify a cursor-opening failure directly.
	result, err := queryInventory(ctx, reader, request.Reference, plan)
	if err != nil {
		return failure, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return failure, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	facts, decodeErr := decodeClusterInterconnects(result, plan, p)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return failure, &Error{Operation: "read", cause: err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ClusterInterconnectsRows transfers the actual caller-native cursor.
func ClusterInterconnectsRows(ctx context.Context, q rowset.Queryer, options ClusterInterconnectsOptions) (*sql.Rows, error) {
	plan, err := clusterInterconnectsPlan(options)
	if err != nil {
		return nil, err
	}
	plan.classifyAccess = true
	return open(ctx, q, plan)
}

// ClusterInterconnectsJSON independently consumes and validates a bounded cursor.
func ClusterInterconnectsJSON(ctx context.Context, q rowset.Queryer, options ClusterInterconnectsOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := clusterInterconnectsPlan(options)
	if err != nil {
		return nil, err
	}
	plan.classifyAccess = true
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion, Complete: true}
	return inventoryJSON(ctx, q, plan, p, limits)
}
