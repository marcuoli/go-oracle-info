package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// DecodeHidden validates a complete visible-underscore result in effective shape.
// Missing requested names are left absent; NULL/default flags are not fabricated.
func DecodeHidden(result session.Result, major int, filter Filter) (EffectiveResult, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	return decodeEffective(result, plan)
}

// ReadHidden independently reads visible underscore instance facts. Report
// nondefault/explicit-name unions belong to callers, including peer comparison.
func ReadHidden(ctx context.Context, reader Reader, request Request) (EffectiveResult, error) {
	plan, err := hiddenPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	facts, decodeErr := decodeEffective(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return EffectiveResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
}

// HiddenRows returns the actual caller-owned cursor for visible underscore
// instance parameters. It does not scan X$ or filter out default-state facts.
func HiddenRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// HiddenJSON is the bounded independent JSON counterpart of HiddenRows. Empty
// selected names remain absent; no unobserved internal defaults are invented.
func HiddenJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// HiddenStatementWithOptions selects the same source as HiddenStatement using one options value.
func HiddenStatementWithOptions(options SourceOptions) (string, []any, error) {
	return HiddenStatement(options.OracleMajor, options.Filter)
}

// DecodeHiddenWithOptions selects the same source as DecodeHidden using one options value.
func DecodeHiddenWithOptions(result session.Result, options SourceOptions) (EffectiveResult, error) {
	return DecodeHidden(result, options.OracleMajor, options.Filter)
}

// HiddenRowsWithOptions selects the same source as HiddenRows using one options value.
func HiddenRowsWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions) (*sql.Rows, error) {
	return HiddenRows(ctx, q, options.OracleMajor, options.Filter)
}

// HiddenJSONWithOptions selects the same source as HiddenJSON using one options value.
func HiddenJSONWithOptions(ctx context.Context, q rowset.Queryer, options SourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	return HiddenJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// HiddenStatement selects only leading-underscore names visible in the same
// instance-effective source and shape as EffectiveStatement. It is not the
// complete internal/default parameter catalog. Report selection stays with callers.
func HiddenStatement(major int, filter Filter) (string, []any, error) {
	plan, err := hiddenPlan(major, filter)
	return plan.sql, plan.args, err
}

func hiddenPlan(major int, filter Filter) (selection, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return selection{}, err
	}
	plan.provenance.NameClass = "leading_underscore_only"
	return build(major, plan.provenance, plan.schema, nil), nil
}
