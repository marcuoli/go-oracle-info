// Package containers reads qualified Oracle container facts through caller-admitted handles.
package containers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Scope is the caller-admitted connected database or container scope.
type Scope string

const (
	Root             Scope = "cdb_root"
	CurrentContainer Scope = "current_container"
	NonCDB           Scope = "non_cdb"
)

// Reader supplies a complete buffered result from an already-admitted handle.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance identifies the selected source and received scope. Complete covers
// the transport only. OptionalColumns lists the optional fields actually queried.
type Provenance struct {
	Source          string   `json:"source"`
	OracleVersion   string   `json:"oracle_version"`
	Scope           Scope    `json:"scope"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
	Complete        bool     `json:"complete"`
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle container source version")
	ErrUnsupportedScope   = errors.New("unsupported Oracle container source scope")
	ErrInvalidRequest     = errors.New("invalid Oracle container request")
	ErrInvalidEvidence    = errors.New("invalid Oracle container evidence")
)

// Outcome classifies a failed observation without claiming missing privilege
// when Oracle reports an ambiguous absent or invisible object.
type Outcome string

const (
	Unavailable  Outcome = "unavailable"
	Inaccessible Outcome = "inaccessible"
	Failed       Outcome = "failed"
)

// ReadError retains its cause for private errors.Is/As checks.
type ReadError struct {
	Source  string
	Outcome Outcome
	cause   error
}

func (e *ReadError) Error() string {
	return "Oracle container read " + string(e.Outcome) + " (" + e.Source + ")"
}
func (e *ReadError) Unwrap() error { return e.cause }

type release int

const (
	release1211 release = iota + 1
	release1212
	release122
)

func profile(version string, scope Scope) (release, error) {
	n, ok := oracleversion.Parse(version)
	if !ok {
		return 0, ErrUnsupportedVersion
	}
	var r release
	switch {
	case n[0] == 12 && n[1] == 1 && n[2] == 0 && n[3] == 1:
		r = release1211
	case n[0] == 12 && n[1] == 1 && n[2] == 0 && n[3] >= 2:
		r = release1212
	case n[0] == 12 && n[1] == 2 && n[2] == 0 && n[3] >= 1:
		r = release122
	case n[0] == 18 || n[0] == 19 || n[0] == 21 || n[0] == 23:
		r = release122
	default:
		return 0, ErrUnsupportedVersion
	}
	switch scope {
	case Root, CurrentContainer:
		return r, nil
	case NonCDB:
		return 0, ErrUnsupportedScope
	default:
		return 0, ErrInvalidRequest
	}
}

type selection struct {
	sql        string
	schema     []rowset.Column
	provenance Provenance
}

type field struct {
	expr   string
	column rowset.Column
}

func projected(name, expr string, kind rowset.Kind) field {
	return field{expr, rowset.Column{Name: name, Kind: kind}}
}

func raw(name string, kind rowset.Kind) field {
	return projected(name, "v."+name, kind)
}

func exact(name string) field {
	return projected(name, "TO_CHAR(v."+name+", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')", rowset.Text)
}

func selectFields(view string, p Provenance, fields, extras []field, predicate, order string) selection {
	columns := make([]rowset.Column, 0, len(fields)+len(extras))
	names := make([]string, 0, cap(columns))
	for _, f := range append(fields, extras...) {
		columns = append(columns, f.column)
		names = append(names, f.expr+" AS "+f.column.Name)
	}
	for _, f := range extras {
		p.OptionalColumns = append(p.OptionalColumns, f.column.Name)
	}
	sql := "SELECT " + strings.Join(names, ", ") + " FROM " + view + " v WHERE " + predicate
	if order != "" {
		sql += " ORDER BY " + order
	}
	return selection{sql: sql, schema: columns, provenance: p}
}

func resultIndex(result session.Result, p selection) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	return index, nil
}

func requiredText(value session.Value) bool { return !value.Null && value.Text != "" }

func unsigned(value session.Value) bool { return value.Null || numeric.IsUnsignedInteger(value.Text) }

func id(value session.Value) int64 {
	n, _ := strconv.ParseInt(value.Text, 10, 64)
	return n
}

func guid(value session.Value) bool {
	if value.Null {
		return true
	}
	if len(value.Text) != 32 {
		return false
	}
	var raw [16]byte
	_, err := hex.Decode(raw[:], []byte(value.Text))
	return err == nil
}

func optional(row []session.Value, index map[string]int, name string) *session.Value {
	i, ok := index[name]
	if !ok {
		return nil
	}
	v := row[i]
	return &v
}

func sourceFailure(source string, cause error, transport bool) error {
	if cause == nil {
		return nil
	}
	outcome := Failed
	if transport {
		cause = projection.ClassifySourceError(cause)
		switch {
		case errors.Is(cause, projection.ErrSourceInaccessible):
			outcome = Inaccessible
		case errors.Is(cause, projection.ErrSourceUnavailable):
			outcome = Unavailable
		}
	}
	return &ReadError{Source: source, Outcome: outcome, cause: cause}
}

func validReader(reader Reader) bool {
	if reader == nil {
		return false
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	}
	return true
}

func readSource(ctx context.Context, reader Reader, p selection, ref reference.Reference) (session.Result, error) {
	if !validReader(reader) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, sourceFailure(p.provenance.Source, err, false)
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: p.sql})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, sourceFailure(p.provenance.Source, err, true)
	}
	return result, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "limits" {
			return cause
		}
		return sourceFailure(p.provenance.Source, cause, stage == "query" || stage == "consume")
	})
}

// ResourceOptions admits the observed release and the connected visibility scope
// for the optional Resource Manager views.
type ResourceOptions struct {
	OracleVersion string
	Scope         Scope
}

func memberPredicate(scope Scope) string {
	if scope == CurrentContainer {
		return "v.CON_ID >= 1 AND v.CON_ID = " + connectedContainer
	}
	return "v.CON_ID >= 1 AND " + connectedContainer + " = 1"
}

// Complete member and source IDs were scanned from the selected native row.
func validMember(instance, container, source session.Value, scope Scope) bool {
	con, reporter := id(container), id(source)
	if id(instance) < 1 || con < 1 {
		return false
	}
	if scope == Root {
		return reporter == 1
	}
	return con == reporter
}

func resourceSelection(options ResourceOptions, source string, fields []field, order string) (selection, error) {
	r, err := profile(options.OracleVersion, options.Scope)
	if err != nil {
		return selection{}, err
	}
	if r < release122 {
		return selection{}, ErrUnsupportedVersion
	}
	p := Provenance{Source: source, OracleVersion: options.OracleVersion, Scope: options.Scope}
	return selectFields(p.Source, p, append([]field{
		raw("INST_ID", rowset.Integer), raw("CON_ID", rowset.Integer),
		projected("SOURCE_CON_ID", connectedContainer, rowset.Integer),
	}, fields...), nil, memberPredicate(options.Scope), order), nil
}

// RootDictionaryOptions admits the observed numeric release and CDB root.
type RootDictionaryOptions struct {
	OracleVersion string
	Scope         Scope
}

const rootDictionaryPredicate = connectedContainer + " = 1"

func rootDictionaryProfile(options RootDictionaryOptions) (release, error) {
	r, err := profile(options.OracleVersion, options.Scope)
	if err != nil {
		return 0, err
	}
	if options.Scope != Root {
		return 0, ErrUnsupportedScope
	}
	return r, nil
}
