package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ApplyCoordinator retains raw process values and SQL NULL. ContainerID nil means
// unprojected on 11g; a selected NULL stays distinct. Counters are cumulative
// since process startup, not throughput, latency or synchronization.
type ApplyCoordinator struct {
	InstanceID, Name, State, Applied, Errors, LowWatermarkMessageNumber session.Value
	ContainerID                                                         *session.Value
}

// ApplyCoordinatorsResult owns admitted rows and source provenance, preserving duplicates.
type ApplyCoordinatorsResult struct {
	Provenance Provenance
	Rows       []ApplyCoordinator
}

func applyCoordinatorsPlan(version string) (selection, error) {
	const messageNumber = "LWM_MESSAGE_NUMBER"
	schema := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "APPLY_NAME", Kind: rowset.Text}, {Name: "STATE", Kind: rowset.Text}, {Name: "TOTAL_APPLIED", Kind: rowset.Integer}, {Name: "TOTAL_ERRORS", Kind: rowset.Integer}, {Name: messageNumber, Kind: rowset.Text}}
	return processSelection(version, "GV$GG_APPLY_COORDINATOR", messageNumber, schema)
}

// ApplyCoordinatorsStatement selects all qualified visible reporting instances without a row limit.
func ApplyCoordinatorsStatement(version string) (string, error) {
	p, e := applyCoordinatorsPlan(version)
	return p.sql, e
}

// DecodeApplyCoordinators validates every row before publishing a slice. Pure decoding
// cannot establish transport completeness; ReadApplyCoordinators sets Complete on success.
func DecodeApplyCoordinators(result session.Result, version string) (ApplyCoordinatorsResult, error) {
	p, err := applyCoordinatorsPlan(version)
	if err != nil {
		return ApplyCoordinatorsResult{}, err
	}
	index, err := columns(result, p)
	if err != nil {
		return ApplyCoordinatorsResult{}, err
	}
	rows := make([]ApplyCoordinator, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ApplyCoordinator{InstanceID: row[index["INST_ID"]], Name: row[index["APPLY_NAME"]], State: row[index["STATE"]], Applied: row[index["TOTAL_APPLIED"]], Errors: row[index["TOTAL_ERRORS"]], LowWatermarkMessageNumber: row[index["LWM_MESSAGE_NUMBER"]], ContainerID: optional(row, index)}
	}
	return ApplyCoordinatorsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadApplyCoordinators executes once over the admitted bounded reader.
// Query, terminal, decoding and cancellation failures publish no successful rows.
func ReadApplyCoordinators(ctx context.Context, r Reader, request Request) (ApplyCoordinatorsResult, error) {
	p, err := applyCoordinatorsPlan(request.OracleVersion)
	if err != nil {
		return ApplyCoordinatorsResult{}, err
	}
	result, err := query(ctx, r, request, p)
	if err != nil {
		return ApplyCoordinatorsResult{Provenance: p.provenance}, err
	}
	value, err := DecodeApplyCoordinators(result, request.OracleVersion)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return ApplyCoordinatorsResult{Provenance: p.provenance}, err
	}
	value.Provenance.Complete = true
	return value, nil
}

// ApplyCoordinatorsRows returns the actual native cursor. Callers own Next/Scan/Err/Close,
// source affinity, deadlines and finite stream budgets.
func ApplyCoordinatorsRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := applyCoordinatorsPlan(version)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

// ApplyCoordinatorsJSON consumes its own cursor through shared rowset lifecycle.
// No JSON survives terminal, close, cancellation, budget or decoding failure.
func ApplyCoordinatorsJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := applyCoordinatorsPlan(version)
	if err != nil {
		return nil, err
	}
	return nativeJSON(ctx, q, p, limits)
}
