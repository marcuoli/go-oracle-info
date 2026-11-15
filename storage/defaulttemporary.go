package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// TemporaryProperty preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// The property can name a temporary tablespace group. It is not guaranteed to match one DBA_TEMP_FILES row; no absence-to-zero inference is made.
type TemporaryProperty struct {
	ReportingInstanceID session.Value
	Name                session.Value
	Value               session.Value
	Description         session.Value
	ContainerID         *session.Value
}

// DefaultTemporaryResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type DefaultTemporaryResult struct {
	Provenance Provenance
	Rows       []TemporaryProperty
}

// DefaultTemporaryStatement selects the reviewed source without SQL row truncation.
func DefaultTemporaryStatement(major int) (string, error) {
	plan, err := defaultTemporaryPlan(major)
	return plan.sql, err
}
func defaultTemporaryPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		{Name: "PROPERTY_NAME", Kind: rowset.Text},
		{Name: "PROPERTY_VALUE", Kind: rowset.Text},
		{Name: "DESCRIPTION", Kind: rowset.Text},
	}
	projection := "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID" + ", " +
		"v.PROPERTY_NAME AS PROPERTY_NAME" + ", " +
		"v.PROPERTY_VALUE AS PROPERTY_VALUE" + ", " +
		"v.DESCRIPTION AS DESCRIPTION"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan := selection{maxRows: 1, schema: schema, provenance: Provenance{Source: "DATABASE_PROPERTIES", OracleMajor: major, Scope: "current_container_dictionary"}}
	where := "v.PROPERTY_NAME = 'DEFAULT_TEMP_TABLESPACE'"
	order := "v.PROPERTY_NAME"
	plan.sql = "SELECT " + projection + " FROM DATABASE_PROPERTIES v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeDefaultTemporary validates every supplied row before publishing any typed slice.
func DecodeDefaultTemporary(result session.Result, major int) (DefaultTemporaryResult, error) {
	plan, err := defaultTemporaryPlan(major)
	if err != nil {
		return DefaultTemporaryResult{}, err
	}
	return decodeDefaultTemporary(result, plan)
}
func decodeDefaultTemporary(result session.Result, plan selection) (DefaultTemporaryResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return DefaultTemporaryResult{}, err
	}
	if len(result.Rows) > 1 {
		return DefaultTemporaryResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
	}
	for _, row := range result.Rows {
		v := row[index["PROPERTY_NAME"]]
		if v.Null || v.Text != "DEFAULT_TEMP_TABLESPACE" {
			return DefaultTemporaryResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence}
		}
	}
	rows := make([]TemporaryProperty, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = TemporaryProperty{
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			Name:                row[index["PROPERTY_NAME"]],
			Value:               row[index["PROPERTY_VALUE"]],
			Description:         row[index["DESCRIPTION"]],
			ContainerID:         optional(row, index, "CON_ID"),
		}
	}
	return DefaultTemporaryResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadDefaultTemporary executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadDefaultTemporary(ctx context.Context, reader Reader, request Request) (DefaultTemporaryResult, error) {
	plan, err := defaultTemporaryPlan(request.OracleMajor)
	if err != nil {
		return DefaultTemporaryResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return DefaultTemporaryResult{}, err
	}
	facts, decodeErr := decodeDefaultTemporary(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return DefaultTemporaryResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DefaultTemporaryRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func DefaultTemporaryRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := defaultTemporaryPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// DefaultTemporaryJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func DefaultTemporaryJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := defaultTemporaryPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeDefaultTemporary(result, plan); return err })
}
