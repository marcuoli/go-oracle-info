package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
)

// JSONEnvelope keeps source/filter provenance separate from ordered native data.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Error avoids formatting driver errors, SQL or bind data. Unwrap retains the
// cause for errors.Is/As and deliberately private diagnostics.
type Error struct {
	Source string
	cause  error
}

// Error returns only the static source, never SQL, bindings or driver text.
func (e *Error) Error() string { return "parameter read failed (" + e.Source + ")" }

// Unwrap retains reported causes for explicit private inspection.
func (e *Error) Unwrap() error { return e.cause }

// EffectiveRows returns the caller's actual native cursor without materializing,
// wrapping or capping its SQL. The caller owns Next/Scan/Err/Close and budgets.
func EffectiveRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// SPFileRows preserves all selected stored clauses, wildcard/defaults and raw
// ISSPECIFIED. Its native cursor has the same ownership as EffectiveRows.
func SPFileRows(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter) (*sql.Rows, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// EffectiveJSON executes EffectiveRows' exact statement/filter on its own
// cursor. It publishes nothing until terminal/close/context/budget checks pass.
func EffectiveJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// SPFileJSON is the independent JSON counterpart of SPFileRows. No value
// precedence, SPFILE-use or physical-file-completeness inference is performed.
func SPFileJSON(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	return rows, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(JSONEnvelope{Provenance: plan.provenance})
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	// RawMessage nil encodes as null; replace those four bytes with bounded data.
	resolved.MaxBytes -= len(metadata) - 4
	if resolved.MaxBytes < 1 {
		return nil, &Error{Source: plan.provenance.Source, cause: rowset.ErrCapacity}
	}
	rows, err := open(ctx, q, plan)
	if err != nil {
		return nil, err
	}
	data, err := rowset.ReadJSON(ctx, rows, plan.schema, resolved)
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	if err = ctx.Err(); err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	payload, err := json.Marshal(JSONEnvelope{Provenance: plan.provenance, Data: data})
	resolved, resolveErr := rowset.Resolve(limits)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	err = errors.Join(err, ctx.Err())
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	return json.RawMessage(payload), nil
}
