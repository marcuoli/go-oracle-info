package services

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"unicode/utf8"

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
	Source          string   `json:"source"`
	Scope           string   `json:"scope"`
	OracleMajor     int      `json:"oracle_major"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid execution reference.
	ErrInvalidRequest = errors.New("invalid Oracle services request")
	// ErrInvalidEvidence rejects malformed metadata, values or late rows.
	ErrInvalidEvidence = errors.New("invalid Oracle services evidence")
	// ErrUnsupportedVersion rejects a release below the reviewed source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle services version")
)

// ReadError retains the original cause for private errors.Is/As inspection.
// Its public message never formats driver values, SQL or bind data.
type ReadError struct{ cause error }

// Error formats only the static domain operation.
func (e *ReadError) Error() string { return "Oracle services fact read failed" }

// Unwrap retains original causes for explicit private inspection.
func (e *ReadError) Unwrap() error { return e.cause }

func validateFactRequest(reader Reader, ref reference.Reference) error {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return ErrInvalidRequest
	}
	return nil
}

func factColumns(result session.Result, schema []rowset.Column) (map[string]int, error) {
	names := make([]string, len(schema))
	for i, column := range schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, column := range schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return nil, ErrInvalidEvidence
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return nil, ErrInvalidEvidence
				}
			}
		}
	}
	return index, nil
}

// Request supplies a known release and a caller-owned execution reference.
// IncludeEdition admits the 11.2.0.2+ column on an established 11g source;
// 12+ always selects it. An unknown 11g patch set leaves it unselected.
type Request struct {
	OracleMajor    int
	IncludeEdition bool
	Reference      reference.Reference
}

// Cancellation is checked after decoding too: a completed payload cannot override it.
func factReadError(ctx context.Context, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &ReadError{cause: err}
	}
	return nil
}
