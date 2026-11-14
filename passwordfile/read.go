package passwordfile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader returns complete bounded results with terminal/close failures retained.
// Its owner supplies admission, lease lifetime, cancellation and transport policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits source options and a valid caller-owned execution reference.
type Request struct {
	Options   Options
	Reference reference.Reference
}

// State separates an observed row, successful empty source and unavailable reads.
type State string

const (
	Available   State = "available"
	NoRows      State = "no_rows"
	Unavailable State = "unavailable"
	Unsupported State = "unsupported"
)

// Info retains observed metadata only, without format/location/file inference.
// IsExascale nil means unselected; a selected SQL NULL has a nonnil Null value.
// ContainerID retains exact integer text; raw format/flag spelling is preserved.
type Info struct {
	FileName    session.Value
	Format      session.Value
	IsASM       session.Value
	IsExascale  *session.Value
	ContainerID session.Value
}

// Observation reports whether the view returned a row, not whether a password file
// exists or was inspected. NoRows never means a verified absent/empty file.
type Observation struct {
	State      State
	Provenance Provenance
	Value      *Info
}

// Error exposes a static operation while retaining causes for errors.Is/As and
// deliberate private diagnostics. Its message never formats a driver/file value.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "password-file " + e.Operation + " failed" }
func (e *Error) Unwrap() error { return e.cause }

// JSONEnvelope separates domain provenance from the ordered native rowset.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Decode validates an entire source observation, retaining successful empty and
// NULL states. The documented view is singular; extra rows are invalid evidence.
func Decode(result session.Result, options Options) (Observation, error) {
	plan, err := selectMetadata(options)
	if err != nil {
		return Observation{State: Unsupported}, err
	}
	return decode(result, plan)
}

// Read queries once through the admitted owned reader. No failed prefix, retry,
// fallback, cache flush or file/credential access occurs here.
func Read(ctx context.Context, reader Reader, request Request) (Observation, error) {
	plan, err := selectMetadata(request.Options)
	if err != nil {
		return Observation{State: Unsupported}, err
	}
	unavailable := Observation{State: Unavailable, Provenance: plan.provenance}
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

// Rows returns the exact caller-owned native cursor; its owner must iterate,
// inspect Err, close on every path and supply stream deadlines/budgets.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := selectMetadata(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// JSON independently reads/finalizes the same selected native source. It returns
// nil on any error/budget failure. All bytes include provenance and escaping.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := selectMetadata(options)
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
	// Oracle documents one row; a second row must not become complete metadata.
	resolved.MaxRows = 1
	rows, err := open(ctx, q, plan)
	if err != nil {
		return nil, err
	}
	data, err := rowset.ReadJSON(ctx, rows, plan.schema, resolved)
	if err != nil {
		return nil, &Error{Operation: "consume", cause: err}
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

func decode(result session.Result, plan selection) (Observation, error) {
	observation := Observation{State: Unavailable, Provenance: plan.provenance}
	fail := func() (Observation, error) {
		return observation, &Error{Operation: "decode", cause: ErrInvalidEvidence}
	}
	names := make([]string, len(plan.schema))
	for i, column := range plan.schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok || len(result.Rows) > 1 {
		return fail()
	}
	if len(result.Rows) == 0 {
		observation.State = NoRows
		return observation, nil
	}
	row := result.Rows[0]
	for _, value := range row {
		if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
			return fail()
		}
	}
	container := row[index["CON_ID"]]
	if !container.Null {
		id, err := strconv.ParseInt(container.Text, 10, 64)
		if err != nil || id != 0 {
			return fail()
		}
	}
	info := &Info{FileName: row[index["FILE_NAME"]], Format: row[index["FORMAT"]], IsASM: row[index["IS_ASM"]], ContainerID: container}
	if position, selected := index["IS_EXC"]; selected {
		value := row[position]
		info.IsExascale = &value
	}
	observation.State, observation.Value = Available, info
	return observation, nil
}
