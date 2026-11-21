package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// HeartbeatLagOptions selects a verified GG_LAG view. Paths is an optional list
// of exact full incoming/outgoing paths, at most 512 bytes each and 1000 entries.
// Group-name substring matching cannot reliably establish path identity.
type HeartbeatLagOptions struct {
	Source HeartbeatSource
	Paths  []string
}

// HeartbeatLagRequest adds the caller-owned SQL reference.
type HeartbeatLagRequest struct {
	Options   HeartbeatLagOptions
	Reference reference.Reference
}

// HeartbeatLag preserves signed NUMBER seconds as exact TM9 text, including
// negative lag caused by clock differences. NULL is unknown, never zero.
// CurrentLocalTimestamp is the view's civil time; DatabaseCurrentUTC is a separate
// explicit UTC database clock. Neither replaces stored heartbeat times or age.
type HeartbeatLag struct {
	LocalDatabase, RemoteDatabase, CurrentLocalTimestamp session.Value
	IncomingAge, IncomingPath, IncomingLag               session.Value
	OutgoingAge, OutgoingPath, OutgoingLag               session.Value
	DatabaseCurrentUTC                                   session.Value
}

// HeartbeatLagResult preserves each directional path and provenance independently.
type HeartbeatLagResult struct {
	Provenance Provenance
	Rows       []HeartbeatLag
}

func heartbeatLagPlan(o HeartbeatLagOptions) (selection, error) {
	if validateNames(o.Paths, 512) != nil {
		return selection{}, ErrInvalidRequest
	}
	p, err := heartbeatDataPlan(o.Source, "GG_LAG")
	if err != nil {
		return selection{}, err
	}
	addDiscoveryFilter(&p, []string{"INCOMING_PATH", "OUTGOING_PATH"}, o.Paths)
	p.sql += " ORDER BY v.LOCAL_DATABASE, v.REMOTE_DATABASE, v.INCOMING_PATH, v.OUTGOING_PATH"
	return p, nil
}

// HeartbeatLagStatement selects exact signed lag/age seconds and raw full paths.
func HeartbeatLagStatement(o HeartbeatLagOptions) (string, []any, error) {
	p, e := heartbeatLagPlan(o)
	return p.sql, p.args, e
}

// DecodeHeartbeatLag validates every source row without float conversion,
// truncation, stale/health classification or a transport-completeness claim.
func DecodeHeartbeatLag(r session.Result, o HeartbeatLagOptions) (HeartbeatLagResult, error) {
	p, err := heartbeatLagPlan(o)
	if err != nil {
		return HeartbeatLagResult{}, err
	}
	idx, err := columns(r, p)
	if err != nil {
		return HeartbeatLagResult{}, err
	}
	rows := make([]HeartbeatLag, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = HeartbeatLag{LocalDatabase: row[idx["LOCAL_DATABASE"]], RemoteDatabase: row[idx["REMOTE_DATABASE"]], CurrentLocalTimestamp: row[idx["CURRENT_LOCAL_TS"]], IncomingAge: row[idx["INCOMING_HEARTBEAT_AGE"]], IncomingPath: row[idx["INCOMING_PATH"]], IncomingLag: row[idx["INCOMING_LAG"]], OutgoingAge: row[idx["OUTGOING_HEARTBEAT_AGE"]], OutgoingPath: row[idx["OUTGOING_PATH"]], OutgoingLag: row[idx["OUTGOING_LAG"]], DatabaseCurrentUTC: row[idx["DB_CURRENT_UTC"]]}
	}
	return HeartbeatLagResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadHeartbeatLag reads independently. A denied or failed component cannot hide
// another successful fact, and separate observations are not an atomic snapshot.
func ReadHeartbeatLag(ctx context.Context, r Reader, req HeartbeatLagRequest) (HeartbeatLagResult, error) {
	p, err := heartbeatLagPlan(req.Options)
	if err != nil {
		return HeartbeatLagResult{}, err
	}
	result, err := query(ctx, r, Request{OracleVersion: p.provenance.OracleVersion, Reference: req.Reference}, p)
	if err != nil {
		return HeartbeatLagResult{}, err
	}
	facts, err := DecodeHeartbeatLag(result, req.Options)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return HeartbeatLagResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// HeartbeatLagRows returns the actual caller-native cursor. Callers own
// Next/Scan/Err/Close, affinity, deadlines and finite streaming budgets.
func HeartbeatLagRows(ctx context.Context, q rowset.Queryer, o HeartbeatLagOptions) (*sql.Rows, error) {
	p, err := heartbeatLagPlan(o)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// HeartbeatLagJSON returns complete bounded exact text and provenance or no bytes.
func HeartbeatLagJSON(ctx context.Context, q rowset.Queryer, o HeartbeatLagOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := heartbeatLagPlan(o)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
