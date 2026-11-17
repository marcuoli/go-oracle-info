package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete bounded results and retains terminal/close errors.
// Its owner admits visibility, physical sessions, cancellation and recovery.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies a known source version and valid caller-owned SQL reference.
type Request struct {
	OracleMajor int
	Reference   reference.Reference
}

// State separates an observed visible set, successful empty set and failed read.
type State string

const (
	Available   State = "available"
	NoRows      State = "no_rows"
	Unavailable State = "unavailable"
)

// Instance retains one source row without collapsing scopes or normalizing flags.
// InstanceID is a validated positive reporting ID. Numeric fields keep exact text;
// ContainerID nil means the column is absent on11g, not a projected SQL NULL.
type Instance struct {
	InstanceID     session.Value
	ContainerID    *session.Value
	InstanceNumber session.Value
	Name           session.Value
	HostName       session.Value
	Status         session.Value
	DatabaseStatus session.Value
	Parallel       session.Value
	Blocked        session.Value
}

// Result is visibility evidence only. Neither Available nor NoRows certifies
// complete physical membership, actual network reachability or parameter coverage.
type Result struct {
	State      State
	Provenance Provenance
	Rows       []Instance
}

// Error prints a static operation, retaining original causes for deliberate
// private inspection. Driver/host/value details never enter its Error message.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "instance visibility " + e.Operation + " failed" }
func (e *Error) Unwrap() error { return e.cause }

// JSONEnvelope keeps source/version/scope apart from ordered native row data.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Decode validates the whole shape/values before returning any member rows.
// Repeated instance observations are preserved, never silently deduplicated.
func Decode(result session.Result, major int) (Result, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return Result{State: Unavailable}, err
	}
	return decode(result, plan)
}

// Read executes one global visibility source. An error discards every returned
// prefix; callers must not substitute the connected instance as complete coverage.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	plan, err := instancePlan(request.OracleMajor)
	if err != nil {
		return Result{State: Unavailable}, err
	}
	unavailable := Result{State: Unavailable, Provenance: plan.provenance}
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return unavailable, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return unavailable, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "query", cause: err}
	}
	facts, decodeErr := decode(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the exact caller-owned native cursor. Its owner supplies streaming
// deadlines/budgets, Next/Scan/Err checks and Close on every path.
func Rows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// JSON executes the same global source independently and delegates complete
// bounded cursor consumption to session/rowset. Any error returns nil JSON.
func JSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return nil, err
	}
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(JSONEnvelope{Provenance: plan.provenance})
	if err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	resolved.MaxBytes -= len(metadata) - 4
	if resolved.MaxBytes < 1 {
		return nil, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	rows, err := open(ctx, q, plan)
	if err != nil {
		return nil, err
	}
	table, err := rowset.Read(ctx, rows, plan.schema, resolved)
	if err != nil {
		return nil, &Error{Operation: "consume", cause: err}
	}
	// A typed rowset is complete transport evidence, not domain admission.
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &Error{Operation: "decode", cause: ErrInvalidEvidence}
	}
	if _, err = columns(result, plan.schema); err != nil {
		return nil, err
	}
	data, err := json.Marshal(table)
	if err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	payload, err := json.Marshal(JSONEnvelope{Provenance: plan.provenance, Data: data})
	original, _ := rowset.Resolve(limits)
	if len(payload) > original.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	return json.RawMessage(payload), nil
}

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, &Error{Operation: "query", cause: err}
	}
	return rows, nil
}

func decode(result session.Result, plan selection) (Result, error) {
	unavailable := Result{State: Unavailable, Provenance: plan.provenance}
	fail := func() (Result, error) { return unavailable, &Error{Operation: "decode", cause: ErrInvalidEvidence} }
	index, err := columns(result, plan.schema)
	if err != nil {
		return fail()
	}
	rows := make([]Instance, len(result.Rows))
	for i, row := range result.Rows {
		record := Instance{InstanceID: row[index["INST_ID"]], InstanceNumber: row[index["INSTANCE_NUMBER"]], Name: row[index["INSTANCE_NAME"]], HostName: row[index["HOST_NAME"]], Status: row[index["STATUS"]], DatabaseStatus: row[index["DATABASE_STATUS"]], Parallel: row[index["PARALLEL"]], Blocked: row[index["BLOCKED"]]}
		if position, present := index["CON_ID"]; present {
			value := row[position]
			record.ContainerID = &value
		}
		rows[i] = record
	}
	state := Available
	if len(rows) == 0 {
		state = NoRows
	}
	return Result{State: state, Provenance: plan.provenance, Rows: rows}, nil
}
