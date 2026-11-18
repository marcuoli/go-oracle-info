package alertlog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

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
// whole-database coverage. An explicit Limit remains a bounded sample.
type Provenance struct {
	Source        string `json:"source"`
	OracleMajor   int    `json:"oracle_major"`
	Scope         string `json:"scope"`
	Limit         int    `json:"sample_limit,omitempty"`
	WindowSeconds int64  `json:"window_seconds,omitempty"`
	Complete      bool   `json:"complete"`
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle alertlog source version")
	ErrInvalidRequest     = errors.New("invalid Oracle alertlog request")
	ErrInvalidEvidence    = errors.New("invalid Oracle alertlog evidence")
)

// Error retains private causes without echoing SQL, bindings or driver diagnostics.
type Error struct {
	Source string
	cause  error
}

func (err *Error) Error() string { return "Oracle alertlog fact read failed (" + err.Source + ")" }
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
func columns(result session.Result, p selection, numeric ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range numeric {
			v := row[index[name]]
			if !v.Null && (v.Text == "" || strings.IndexFunc(v.Text, func(r rune) bool { return r < '0' || r > '9' }) >= 0) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	p.provenance.Complete = true
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: p.provenance}
	overhead, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(overhead) - 4
	if budget.MaxBytes < 1 {
		return nil, &Error{p.provenance.Source, rowset.ErrCapacity}
	}
	cursor, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	table, err := rowset.Read(ctx, cursor, p.schema, budget)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	if err = validate(result); err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	envelope.Data, err = json.Marshal(table)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = readError(ctx, p, err); err != nil {
		return nil, err
	}
	return payload, nil
}
