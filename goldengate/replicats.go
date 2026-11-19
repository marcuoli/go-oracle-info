package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Replicat retains the external group, database inbound server and raw attachment
// status separately. ATTACHED does not establish an external RUNNING verdict.
type Replicat struct{ Name, ServerName, ApplyUser, Status session.Value }

// ReplicatsResult is the complete selected admitted dictionary, not all deployments.
type ReplicatsResult struct {
	Provenance Provenance
	Rows       []Replicat
}

func replicatsPlan(o ProcessOptions) (selection, error) {
	source, err := processSource(o, "GOLDENGATE_INBOUND")
	if err != nil {
		return selection{}, err
	}
	p := selection{sql: "SELECT v.REPLICAT_NAME AS REPLICAT_NAME, v.SERVER_NAME AS SERVER_NAME, v.APPLY_USER AS APPLY_USER, v.STATUS AS STATUS FROM " + source + " v", schema: []rowset.Column{{Name: "REPLICAT_NAME", Kind: rowset.Text}, {Name: "SERVER_NAME", Kind: rowset.Text}, {Name: "APPLY_USER", Kind: rowset.Text}, {Name: "STATUS", Kind: rowset.Text}}, provenance: Provenance{Source: source, Scope: "admitted_container_dictionary", OracleVersion: o.OracleVersion}, required: []string{"SERVER_NAME"}}
	addDiscoveryFilter(&p, []string{"REPLICAT_NAME"}, o.Names)
	p.sql += " ORDER BY v.SERVER_NAME, v.REPLICAT_NAME"
	return p, nil
}

// ReplicatsStatement selects integrated Replicat identity and database attachment.
func ReplicatsStatement(o ProcessOptions) (string, []any, error) {
	p, e := replicatsPlan(o)
	return p.sql, p.args, e
}

// DecodeReplicats preserves NULL/missing group identity without substituting a
// server/user name, and validates all rows before publishing a slice.
func DecodeReplicats(r session.Result, o ProcessOptions) (ReplicatsResult, error) {
	p, err := replicatsPlan(o)
	if err != nil {
		return ReplicatsResult{}, err
	}
	idx, err := columns(r, p)
	if err != nil {
		return ReplicatsResult{}, err
	}
	rows := make([]Replicat, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = Replicat{Name: row[idx["REPLICAT_NAME"]], ServerName: row[idx["SERVER_NAME"]], ApplyUser: row[idx["APPLY_USER"]], Status: row[idx["STATUS"]]}
	}
	return ReplicatsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadReplicats performs one independent complete read. A denial does not erase
// successful evidence from a separate Extract or heartbeat read owned by callers.
func ReadReplicats(ctx context.Context, r Reader, req ProcessRequest) (ReplicatsResult, error) {
	p, err := replicatsPlan(req.Options)
	if err != nil {
		return ReplicatsResult{}, err
	}
	result, err := query(ctx, r, Request{OracleVersion: req.Options.OracleVersion, Reference: req.Reference}, p)
	if err != nil {
		return ReplicatsResult{}, err
	}
	facts, err := DecodeReplicats(result, req.Options)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return ReplicatsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ReplicatsRows returns the actual caller-native cursor; the caller owns
// Next/Scan/Err/Close, source affinity, deadlines and finite streaming budgets.
func ReplicatsRows(ctx context.Context, q rowset.Queryer, o ProcessOptions) (*sql.Rows, error) {
	p, err := replicatsPlan(o)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// ReplicatsJSON independently publishes a complete bounded source or no bytes.
func ReplicatsJSON(ctx context.Context, q rowset.Queryer, o ProcessOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := replicatsPlan(o)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
