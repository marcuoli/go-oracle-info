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

// HeartbeatsOptions selects a verified GG_HEARTBEAT object. Process lists select
// a row containing matching incoming or outgoing identities; values stay exact.
type HeartbeatsOptions struct {
	Source              HeartbeatSource
	Extracts, Replicats []string
}

// HeartbeatsRequest supplies the admitted source and caller-owned SQL reference.
type HeartbeatsRequest struct {
	Options   HeartbeatsOptions
	Reference reference.Reference
}

// Heartbeat retains distinct directional identities and stored UTC civil times.
// DatabaseCurrentUTC is query time, never the time of last replication progress.
// Incoming/OutgoingRoutingPath is preserved without parsing pump/path grammar.
type Heartbeat struct {
	LocalDatabase, RemoteDatabase, Timestamp                                  session.Value
	IncomingExtract, IncomingRoutingPath, IncomingReplicat, IncomingTimestamp session.Value
	OutgoingExtract, OutgoingRoutingPath, OutgoingReplicat, OutgoingTimestamp session.Value
	DatabaseCurrentUTC                                                        session.Value
}

// HeartbeatsResult retains raw source observations without stale/health policy.
type HeartbeatsResult struct {
	Provenance Provenance
	Rows       []Heartbeat
}

type heartbeatField struct{ name, dataType string }

func heartbeatFields(name string) []heartbeatField {
	if name == "GG_HEARTBEAT" {
		return []heartbeatField{
			{"LOCAL_DATABASE", "VARCHAR2"}, {"REMOTE_DATABASE", "VARCHAR2"}, {"HEARTBEAT_TIMESTAMP", "TIMESTAMP(6)"},
			{"INCOMING_EXTRACT", "VARCHAR2"}, {"INCOMING_ROUTING_PATH", "VARCHAR2"}, {"INCOMING_REPLICAT", "VARCHAR2"}, {"INCOMING_HEARTBEAT_TS", "TIMESTAMP(6)"},
			{"OUTGOING_EXTRACT", "VARCHAR2"}, {"OUTGOING_ROUTING_PATH", "VARCHAR2"}, {"OUTGOING_REPLICAT", "VARCHAR2"}, {"OUTGOING_HEARTBEAT_TS", "TIMESTAMP(6)"},
		}
	}
	if name == "GG_LAG" {
		return []heartbeatField{
			{"LOCAL_DATABASE", "VARCHAR2"}, {"REMOTE_DATABASE", "VARCHAR2"}, {"CURRENT_LOCAL_TS", "TIMESTAMP(6)"},
			{"INCOMING_HEARTBEAT_AGE", "NUMBER"}, {"INCOMING_PATH", "VARCHAR2"}, {"INCOMING_LAG", "NUMBER"},
			{"OUTGOING_HEARTBEAT_AGE", "NUMBER"}, {"OUTGOING_PATH", "VARCHAR2"}, {"OUTGOING_LAG", "NUMBER"},
		}
	}
	return nil
}

func heartbeatDataPlan(s HeartbeatSource, name string) (selection, error) {
	if s.name != name || !identifier(s.owner) {
		return selection{}, ErrInvalidRequest
	}
	if !discoveryVersion(s.version) {
		return selection{}, ErrUnsupportedVersion
	}
	p := selection{provenance: Provenance{Source: name, ObjectOwner: s.owner, ObjectName: name, Scope: "admitted_verified_object_current_container", OracleVersion: s.version}, required: []string{"DB_CURRENT_UTC"}}
	expressions := []string{}
	for _, field := range heartbeatFields(name) {
		expression := "v." + field.name
		switch field.dataType {
		case "TIMESTAMP(6)":
			expression = heartbeatTimestamp(expression)
			p.timestamps = append(p.timestamps, field.name)
		case "NUMBER":
			expression = "TO_CHAR(" + expression + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
			p.decimals = append(p.decimals, field.name)
		}
		p.schema = append(p.schema, rowset.Column{Name: field.name, Kind: rowset.Text})
		expressions = append(expressions, expression+" AS "+field.name)
	}
	p.schema = append(p.schema, rowset.Column{Name: "DB_CURRENT_UTC", Kind: rowset.Text})
	p.timestamps = append(p.timestamps, "DB_CURRENT_UTC")
	expressions = append(expressions, heartbeatTimestamp("SYS_EXTRACT_UTC(SYSTIMESTAMP)")+" AS DB_CURRENT_UTC")
	p.sql = "SELECT " + strings.Join(expressions, ", ") + " FROM " + quoteIdentifier(s.owner) + "." + quoteIdentifier(name) + " v"
	return p, nil
}

func heartbeatsPlan(o HeartbeatsOptions) (selection, error) {
	if ValidateDiscoveryFilter(DiscoveryFilter{Extracts: o.Extracts, Replicats: o.Replicats}) != nil {
		return selection{}, ErrInvalidRequest
	}
	p, err := heartbeatDataPlan(o.Source, "GG_HEARTBEAT")
	if err != nil {
		return selection{}, err
	}
	addDiscoveryFilter(&p, []string{"INCOMING_EXTRACT", "OUTGOING_EXTRACT"}, o.Extracts)
	addDiscoveryFilter(&p, []string{"INCOMING_REPLICAT", "OUTGOING_REPLICAT"}, o.Replicats)
	p.sql += " ORDER BY v.LOCAL_DATABASE, v.REMOTE_DATABASE, v.INCOMING_EXTRACT, v.INCOMING_ROUTING_PATH, v.INCOMING_REPLICAT, v.OUTGOING_EXTRACT, v.OUTGOING_ROUTING_PATH, v.OUTGOING_REPLICAT, v.HEARTBEAT_TIMESTAMP"
	return p, nil
}

// HeartbeatsStatement admits only a structurally verified fixed-name source;
// owner identifiers are quoted, process values bound and no schema is guessed.
func HeartbeatsStatement(o HeartbeatsOptions) (string, []any, error) {
	p, e := heartbeatsPlan(o)
	return p.sql, p.args, e
}

// DecodeHeartbeats validates every row and retains SQL NULLs and exact UTC text.
// Stored times, not a new SELECT time, establish heartbeat freshness evidence.
func DecodeHeartbeats(r session.Result, o HeartbeatsOptions) (HeartbeatsResult, error) {
	p, err := heartbeatsPlan(o)
	if err != nil {
		return HeartbeatsResult{}, err
	}
	idx, err := columns(r, p)
	if err != nil {
		return HeartbeatsResult{}, err
	}
	rows := make([]Heartbeat, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = Heartbeat{LocalDatabase: row[idx["LOCAL_DATABASE"]], RemoteDatabase: row[idx["REMOTE_DATABASE"]], Timestamp: row[idx["HEARTBEAT_TIMESTAMP"]], IncomingExtract: row[idx["INCOMING_EXTRACT"]], IncomingRoutingPath: row[idx["INCOMING_ROUTING_PATH"]], IncomingReplicat: row[idx["INCOMING_REPLICAT"]], IncomingTimestamp: row[idx["INCOMING_HEARTBEAT_TS"]], OutgoingExtract: row[idx["OUTGOING_EXTRACT"]], OutgoingRoutingPath: row[idx["OUTGOING_ROUTING_PATH"]], OutgoingReplicat: row[idx["OUTGOING_REPLICAT"]], OutgoingTimestamp: row[idx["OUTGOING_HEARTBEAT_TS"]], DatabaseCurrentUTC: row[idx["DB_CURRENT_UTC"]]}
	}
	return HeartbeatsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadHeartbeats reads one complete source. It changes no cursor owner/scope and
// a source race, denial or malformed late row never becomes an empty success.
func ReadHeartbeats(ctx context.Context, r Reader, req HeartbeatsRequest) (HeartbeatsResult, error) {
	p, err := heartbeatsPlan(req.Options)
	if err != nil {
		return HeartbeatsResult{}, err
	}
	result, err := query(ctx, r, Request{OracleVersion: p.provenance.OracleVersion, Reference: req.Reference}, p)
	if err != nil {
		return HeartbeatsResult{}, err
	}
	facts, err := DecodeHeartbeats(result, req.Options)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return HeartbeatsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// HeartbeatsRows returns the actual caller cursor; callers own Next/Scan/Err/Close,
// budgets, deadlines and the verified object's endpoint/container affinity.
func HeartbeatsRows(ctx context.Context, q rowset.Queryer, o HeartbeatsOptions) (*sql.Rows, error) {
	p, err := heartbeatsPlan(o)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// HeartbeatsJSON independently returns complete bounded data with owner provenance.
func HeartbeatsJSON(ctx context.Context, q rowset.Queryer, o HeartbeatsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := heartbeatsPlan(o)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
