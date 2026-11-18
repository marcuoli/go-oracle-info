package goldengate

import (
	"context"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// SchemaState describes configured-schema evidence independently of privileges.
type SchemaState string

// SchemaUnavailable means the admitted database source cannot establish GGSCHEMA.
const SchemaUnavailable SchemaState = "unavailable"

// SchemaObservation never promotes an administrator username or an object-owner
// guess to the externally configured GGSCHEMA. Name nil means unobserved configuration, not a selected SQL NULL.
type SchemaObservation struct {
	State  SchemaState
	Name   *string
	Reason string
}

// Properties keeps configured schema evidence separate from administrator grants.
// A complete empty administrator result does not prove GoldenGate absent.
type Properties struct {
	ConfiguredSchema SchemaObservation
	Administrators   AdministratorsResult
}

// ReadProperties collects database-only facts with the caller's statement/result
// contract. ConfiguredSchema remains unavailable, including when one admin exists.
func ReadProperties(ctx context.Context, reader Reader, request Request) (Properties, error) {
	facts := Properties{ConfiguredSchema: SchemaObservation{State: SchemaUnavailable, Reason: "not_exposed_by_database_source"}}
	administrators, err := ReadAdministrators(ctx, reader, request)
	facts.Administrators = administrators
	return facts, err
}

// ReadNativeProperties borrows a caller-admitted Queryer and delegates buffering
// and finalization to shared rowset. It owns no connection, container or retry.
// The SQL reference remains caller-supplied, as for ReadProperties.
func ReadNativeProperties(ctx context.Context, q rowset.Queryer, request Request, limits rowset.Limits) (Properties, error) {
	p, err := administratorsPlan(request.OracleVersion)
	if err != nil {
		return Properties{ConfiguredSchema: SchemaObservation{State: SchemaUnavailable, Reason: "not_exposed_by_database_source"}}, err
	}
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return Properties{}, err
	}
	return ReadProperties(ctx, nativeAdministratorReader{q: q, plan: p, limits: resolved}, request)
}

type nativeAdministratorReader struct {
	q      rowset.Queryer
	plan   selection
	limits rowset.Limits
}

func (r nativeAdministratorReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	rows, err := rowset.Query(ctx, r.q, s.Text, s.Args...)
	if err != nil {
		return session.Result{}, err
	}
	table, err := rowset.Read(ctx, rows, r.plan.schema, r.limits)
	if err != nil {
		return session.Result{}, err
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return session.Result{}, ErrInvalidEvidence
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, err
	}
	return result, nil
}
