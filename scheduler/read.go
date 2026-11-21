package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Reader supplies complete finite-budget results, including terminal/close errors.
// Its caller owns connections, admission, deadlines and recovery.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage.
type Provenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
	Owner       string `json:"owner,omitempty"`
	Complete    bool   `json:"complete"`
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle scheduler source version")
	ErrInvalidRequest     = errors.New("invalid Oracle scheduler request")
	ErrInvalidEvidence    = errors.New("invalid Oracle scheduler evidence")
)

// Error retains private causes without echoing SQL, bindings or driver diagnostics.
type Error struct {
	Source string
	cause  error
}

func (err *Error) Error() string { return "Oracle scheduler fact read failed (" + err.Source + ")" }
func (err *Error) Unwrap() error { return err.cause }

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance Provenance
}

// New fact projections admit the independently reviewed numeric release families.
// Older compatibility SQL remains available; branding does not admit numeric26.
func admittedMajor(major int) error {
	switch major {
	case 19, 21, 23:
		return nil
	}
	return ErrUnsupportedVersion
}
func validReader(reader Reader) bool {
	if reader == nil {
		return false
	}
	v := reflect.ValueOf(reader)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}
func readError(ctx context.Context, p selection, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &Error{p.provenance.Source, err}
	}
	return nil
}
func columns(result session.Result, p selection, integerFields ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range integerFields {
			v := row[index[name]]
			if !v.Null && !numeric.IsUnsignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{p.provenance.Source, cause}
	}, "limits", "header")
}
