package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Extract retains the external client name separately from its database capture
// name. ClientStatus is attachment evidence, not external process health.
type Extract struct {
	CaptureName, ClientName, CaptureUser, Status, ClientStatus, Purpose, SourceDatabase session.Value
}

// ExtractsResult describes only the selected admitted dictionary visibility.
type ExtractsResult struct {
	Provenance Provenance
	Rows       []Extract
}

func extractsPlan(o ProcessOptions) (selection, error) {
	source, err := processSource(o, "CAPTURE")
	if err != nil {
		return selection{}, err
	}
	p := selection{provenance: Provenance{Source: source, Scope: "admitted_container_dictionary", OracleVersion: o.OracleVersion}, required: []string{"CAPTURE_NAME", "PURPOSE"}, ggPurpose: true}
	names := []string{"CAPTURE_NAME", "CLIENT_NAME", "CAPTURE_USER", "STATUS", "CLIENT_STATUS", "PURPOSE", "SOURCE_DATABASE"}
	p.sql = "SELECT "
	for i, name := range names {
		if i > 0 {
			p.sql += ", "
		}
		p.sql += "v." + name + " AS " + name
		p.schema = append(p.schema, rowset.Column{Name: name, Kind: rowset.Text})
	}
	p.sql += " FROM " + source + " v WHERE v.PURPOSE = 'GoldenGate Capture'"
	addDiscoveryFilter(&p, []string{"CLIENT_NAME"}, o.Names)
	p.sql += " ORDER BY v.CAPTURE_NAME, v.CLIENT_NAME"
	return p, nil
}

// ExtractsStatement selects integrated GoldenGate captures, excluding XStream
// and Audit Vault. Names selects CLIENT_NAME, never the internal capture name.
func ExtractsStatement(o ProcessOptions) (string, []any, error) {
	p, e := extractsPlan(o)
	return p.sql, p.args, e
}

// DecodeExtracts validates every supplied row without claiming transport completion.
// A NULL client name remains unavailable identity, never the capture name.
func DecodeExtracts(r session.Result, o ProcessOptions) (ExtractsResult, error) {
	p, err := extractsPlan(o)
	if err != nil {
		return ExtractsResult{}, err
	}
	idx, err := columns(r, p)
	if err != nil {
		return ExtractsResult{}, err
	}
	rows := make([]Extract, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = Extract{CaptureName: row[idx["CAPTURE_NAME"]], ClientName: row[idx["CLIENT_NAME"]], CaptureUser: row[idx["CAPTURE_USER"]], Status: row[idx["STATUS"]], ClientStatus: row[idx["CLIENT_STATUS"]], Purpose: row[idx["PURPOSE"]], SourceDatabase: row[idx["SOURCE_DATABASE"]]}
	}
	return ExtractsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadExtracts reads once, preserving denied/failed errors rather than returning
// an empty success or retrying with a different visibility.
func ReadExtracts(ctx context.Context, r Reader, req ProcessRequest) (ExtractsResult, error) {
	p, err := extractsPlan(req.Options)
	if err != nil {
		return ExtractsResult{}, err
	}
	result, err := query(ctx, r, Request{OracleVersion: req.Options.OracleVersion, Reference: req.Reference}, p)
	if err != nil {
		return ExtractsResult{}, err
	}
	facts, err := DecodeExtracts(result, req.Options)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return ExtractsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ExtractsRows returns the actual caller-native cursor. The caller retains
// Next/Scan/Err/Close, handle affinity, deadlines and finite streaming budgets.
func ExtractsRows(ctx context.Context, q rowset.Queryer, o ProcessOptions) (*sql.Rows, error) {
	p, err := extractsPlan(o)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// ExtractsJSON independently consumes a complete bounded source or returns no bytes.
func ExtractsJSON(ctx context.Context, q rowset.Queryer, o ProcessOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := extractsPlan(o)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
