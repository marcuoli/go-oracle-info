package instances

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// columns validates the complete visible-instance source before either decoder
// publishes rows. Mapping, provenance and operation-specific errors stay with
// the caller; shared validation keeps visibility and version admission aligned.
func columns(result session.Result, schema []rowset.Column) (map[string]int, error) {
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
		id := row[index["INST_ID"]]
		number, err := strconv.ParseInt(id.Text, 10, 64)
		if id.Null || err != nil || number < 1 {
			return nil, ErrInvalidEvidence
		}
	}
	return index, nil
}

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

// Error prints a static operation, retaining original causes for deliberate
// private inspection. Driver/host/value details never enter its Error message.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "instance visibility " + e.Operation + " failed" }

func (e *Error) Unwrap() error { return e.cause }

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, &Error{Operation: "query", cause: err}
	}
	return rows, nil
}

// Provenance identifies only the admitted dynamic source and selected scope.
// Qualified visible rows do not prove every configured or reachable member.
type Provenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
}

var (
	// ErrUnsupportedVersion rejects a source below the reviewed 11g floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle instance visibility version")
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("instance reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed source metadata, IDs or values.
	ErrInvalidEvidence = errors.New("invalid Oracle instance visibility evidence")
)

type selection struct {
	sql        string
	schema     []rowset.Column
	provenance Provenance
}
