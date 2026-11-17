package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Capture retains raw process values and SQL NULL. ContainerID nil means
// unprojected on 11g; a selected NULL stays distinct. Counters are cumulative
// since process startup, not throughput, latency or synchronization.
type Capture struct {
	InstanceID, Name, State, CapturedMessageNumber, MessagesCaptured session.Value
	ContainerID                                                      *session.Value
}

// CaptureResult owns admitted rows and source provenance, preserving duplicates.
type CaptureResult struct {
	Provenance Provenance
	Rows       []Capture
}

// CaptureStatement selects all qualified visible reporting instances without a row limit.
func CaptureStatement(version string) (string, error) { p, e := plan(version, false); return p.sql, e }

// DecodeCapture validates every row before publishing a slice. Pure decoding
// cannot establish transport completeness; ReadCapture sets Complete on success.
func DecodeCapture(result session.Result, version string) (CaptureResult, error) {
	p, err := plan(version, false)
	if err != nil {
		return CaptureResult{}, err
	}
	index, err := columns(result, p)
	if err != nil {
		return CaptureResult{}, err
	}
	rows := make([]Capture, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Capture{InstanceID: row[index["INST_ID"]], Name: row[index["CAPTURE_NAME"]], State: row[index["STATE"]], CapturedMessageNumber: row[index["CAPTURE_MESSAGE_NUMBER"]], MessagesCaptured: row[index["TOTAL_MESSAGES_CAPTURED"]], ContainerID: optional(row, index)}
	}
	return CaptureResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadCapture executes once over the admitted bounded reader.
// Query, terminal, decoding and cancellation failures publish no successful rows.
func ReadCapture(ctx context.Context, r Reader, request Request) (CaptureResult, error) {
	p, err := plan(request.OracleVersion, false)
	if err != nil {
		return CaptureResult{}, err
	}
	result, err := query(ctx, r, request, p)
	if err != nil {
		return CaptureResult{Provenance: p.provenance}, err
	}
	value, err := DecodeCapture(result, request.OracleVersion)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return CaptureResult{Provenance: p.provenance}, err
	}
	value.Provenance.Complete = true
	return value, nil
}

// CaptureRows returns the actual native cursor. Callers own Next/Scan/Err/Close,
// source affinity, deadlines and finite stream budgets.
func CaptureRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := plan(version, false)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

// CaptureJSON consumes its own cursor through shared rowset lifecycle.
// No JSON survives terminal, close, cancellation, budget or decoding failure.
func CaptureJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := plan(version, false)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
