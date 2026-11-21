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
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, plan)
	}, plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(&Error{Operation: "decode", cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := columns(result, plan.schema)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "header", "encode":
			return &Error{Operation: "encode", cause: cause}
		case "budget", "consume":
			return &Error{Operation: stage, cause: cause}
		}
		return cause
	})
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

// InventoryOptions admits the Oracle projection for visible instances.
type InventoryOptions struct {
	OracleMajor int
}

// InventoryRequest adds the caller's SQL reference to the source options.
type InventoryRequest struct {
	Options   InventoryOptions
	Reference reference.Reference
}

// InventoryStatement is the named form of Statement.
func InventoryStatement(options InventoryOptions) (string, error) {
	return Statement(options.OracleMajor)
}

// DecodeInventory is the named form of Decode.
func DecodeInventory(result session.Result, options InventoryOptions) (Result, error) {
	return Decode(result, options.OracleMajor)
}

// ReadInventory is the named form of Read.
func ReadInventory(ctx context.Context, reader Reader, request InventoryRequest) (Result, error) {
	return Read(ctx, reader, Request{OracleMajor: request.Options.OracleMajor, Reference: request.Reference})
}

// InventoryRows is the named form of Rows.
func InventoryRows(ctx context.Context, q rowset.Queryer, options InventoryOptions) (*sql.Rows, error) {
	return Rows(ctx, q, options.OracleMajor)
}

// InventoryJSON is the named form of JSON.
func InventoryJSON(ctx context.Context, q rowset.Queryer, options InventoryOptions, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options.OracleMajor, limits)
}
