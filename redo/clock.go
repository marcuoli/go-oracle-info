package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"time"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ClockResult retains one observed SYSDATE civil label and its connected-source
// identity. LocalTime has whole seconds and no timezone or UTC interpretation.
// ContainerID nil means unprojected on 11g; selected SQL NULL stays distinct.
// This independent observation does not attest clock synchronization across RAC.
type ClockResult struct {
	Provenance          Provenance
	LocalTime           session.Value
	ReportingInstanceID session.Value
	ContainerID         *session.Value
}

// ClockStatement selects the connected source's SYSDATE through the Gregorian
// civil renderer without changing NLS/session settings or appending a timezone.
func ClockStatement(major int) (string, error) { plan, err := clockPlan(major); return plan.sql, err }

func clockPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	plan := selection{schema: []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "LOCAL_TIME", Kind: rowset.Text}}, provenance: Provenance{Source: "SYSDATE", OracleMajor: major, Scope: "connected_instance_clock"}}
	plan.sql = "SELECT TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS INST_ID, " + datetime.GregorianDate("SYSDATE") + " AS LOCAL_TIME"
	if major >= 12 {
		plan.schema = append(plan.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		plan.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS CON_ID"
	}
	plan.sql += " FROM SYS.DUAL"
	return plan, nil
}

// DecodeClock admits exactly one non-NULL whole-second local civil clock label.
// A pure decoder validates evidence but cannot prove transport completion.
func DecodeClock(result session.Result, major int) (ClockResult, error) {
	plan, err := clockPlan(major)
	if err != nil {
		return ClockResult{}, err
	}
	return decodeClock(result, plan)
}

func decodeClock(result session.Result, plan selection) (ClockResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ClockResult{}, err
	}
	fail := func() (ClockResult, error) { return ClockResult{}, &Error{plan.provenance.Source, ErrInvalidEvidence} }
	if len(result.Rows) != 1 {
		return fail()
	}
	row := result.Rows[0]
	value := row[index["LOCAL_TIME"]]
	parsed, err := time.Parse("2006-01-02T15:04:05", value.Text)
	if value.Null || err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02T15:04:05") != value.Text {
		return fail()
	}
	return ClockResult{Provenance: plan.provenance, LocalTime: value, ReportingInstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID")}, nil
}

// ReadClock queries the already admitted complete bounded reader once. Instance
// filters are rejected because this fact describes only the connected source.
// The caller keeps deadlines, source affinity and collection-UTC timestamps.
func ReadClock(ctx context.Context, reader Reader, request Request) (ClockResult, error) {
	if len(request.Filter.Instances) != 0 {
		return ClockResult{}, ErrInvalidRequest
	}
	plan, err := clockPlan(request.OracleMajor)
	if err != nil {
		return ClockResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return ClockResult{}, err
	}
	clock, decodeErr := decodeClock(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ClockResult{}, &Error{plan.provenance.Source, err}
	}
	clock.Provenance.Complete = true
	return clock, nil
}

// ClockRows returns the actual caller-native SYSDATE cursor. Its owner retains
// iteration, exact one-row/value admission, Err/Close, deadlines and budgets.
func ClockRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := clockPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ClockJSON completely consumes one shared rowset, applies DecodeClock admission
// and encodes the original native table. Any failure publishes nil JSON.
func ClockJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := clockPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeClock(result, plan); return err })
}
