package memory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader is an already-owned, bounded query interface. Its Result must be complete
// after terminal iteration/close checks; capacity errors must not return a prefix.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies admitted scope and distinct caller-namespaced SQL references.
// Known Oracle 18+ connected PDBs require a second reference for the guarded
// effective query; other contexts do not evaluate that SQL.
type Request struct {
	Parameter          Parameter
	Options            Options
	PrimaryReference   reference.Reference
	EffectiveReference reference.Reference
}

// ReadError retains a programmatic cause without exposing driver/bind text.
type ReadError struct {
	Parameter Parameter
	Effective bool
	cause     error
}

func (e *ReadError) Error() string {
	if e.Effective {
		return "configured-memory effective-source query failed"
	}
	return "configured-memory primary-source query failed"
}
func (e *ReadError) Unwrap() error { return e.cause }

// Collect performs the selected read and, only when eligible, its effective ADB
// source fallback. It neither reconnects nor retries a failed statement. A primary
// observation remains returned if a later fallback fails, with that failure intact.
func Collect(ctx context.Context, reader Reader, request Request) (Observation, error) {
	if reader == nil {
		return Observation{}, errors.New("configured-memory reader is required")
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	o := request.Options
	if o.PDB.EffectiveADB {
		return Observation{}, ErrUnsupportedScope
	}
	statement, err := Statement(request.Parameter, o)
	if err != nil {
		return Observation{}, err
	}
	eligible := o.Scope == ConnectedPDB && o.OracleMajor >= 18
	if request.PrimaryReference.Namespace() == "" || (eligible && (request.EffectiveReference.Namespace() == "" || request.PrimaryReference == request.EffectiveReference)) {
		return Observation{}, errors.New("configured-memory execution references are missing or not distinct")
	}
	var args []any
	if o.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", o.TargetContainerID)}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.PrimaryReference, Text: statement, Args: args})
	if err != nil && !onlyNoRows(err) {
		return Observation{}, &ReadError{Parameter: request.Parameter, cause: err}
	}
	observation := Observation{State: NoRows}
	if err == nil {
		observation, err = Decode(result, request.Parameter, o)
	}
	if onlyNoRows(err) {
		err = nil
	}
	if err != nil {
		return Observation{}, err
	}
	// A complete read can race cancellation. Keep validated primary evidence,
	// but never report it as a successful collection after cancellation.
	if err := ctx.Err(); err != nil {
		return observation, err
	}
	if !eligible || (observation.State != Null && observation.State != NoRows) {
		return observation, nil
	}
	o.PDB.EffectiveADB = true
	statement, err = Statement(request.Parameter, o)
	if err != nil {
		return observation, err
	}
	result, err = reader.Query(ctx, session.Statement{Reference: request.EffectiveReference, Text: statement})
	missing := onlyNoRows(err)
	if missing {
		err = nil
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return observation, &ReadError{Parameter: request.Parameter, Effective: true, cause: err}
	}
	if missing {
		return observation, nil
	}
	effective, err := Decode(result, request.Parameter, o)
	if cancelled := ctx.Err(); cancelled != nil {
		return observation, errors.Join(err, cancelled)
	}
	if err != nil {
		return observation, err
	}
	// An ineligible/empty effective source must not erase a primary NULL's
	// validated provenance. A NULL effective row can enrich a primary no-row read.
	if effective.State == Available || (observation.State == NoRows && effective.State == Null) {
		return effective, nil
	}
	return observation, nil
}

// ErrNoRows can coexist with a terminal failure in an error tree. Normalize it
// only when every cause is a no-row outcome, including ordinary wrapped adapters.
func onlyNoRows(err error) bool {
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		causes := cause.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, child := range causes {
			if !onlyNoRows(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return onlyNoRows(cause.Unwrap())
	default:
		return errors.Is(err, sql.ErrNoRows)
	}
}
