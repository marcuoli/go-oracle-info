package database

import (
	"context"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader returns only complete bounded results and retains terminal/close failures.
// The caller owns admission, connections, cancellation and retry policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance records the selected source and admitted visibility, not physical coverage.
type Provenance struct {
	Source      string `json:"source"`
	Scope       string `json:"scope"`
	OracleMajor int    `json:"oracle_major"`
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid execution reference.
	ErrInvalidRequest = errors.New("invalid Oracle database request")
	// ErrInvalidEvidence rejects malformed metadata, values or late rows.
	ErrInvalidEvidence = errors.New("invalid Oracle database evidence")
	// ErrUnsupportedVersion rejects a release below the reviewed source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle database version")
)

// ReadError retains the original cause for private errors.Is/As inspection.
// Its public message never formats driver values, SQL or bind data.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle database fact read failed" }

// Unwrap retains original causes for explicit private inspection.
func (e *ReadError) Unwrap() error { return e.cause }

func validateFactRequest(reader Reader, ref reference.Reference) error {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return ErrInvalidRequest
	}
	return nil
}

func factColumns(result session.Result, schema []rowset.Column) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	return index, nil
}

// Cancellation is checked after decoding too: a completed payload cannot override it.
func factReadError(ctx context.Context, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &ReadError{cause: err}
	}
	return nil
}
