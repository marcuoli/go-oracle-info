// Package dataguard exposes admitted read-only transport and apply observations.
package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// DatasetProvenance records the admitted source, numeric release and visibility.
// Complete is selected-transport completion, never physical RAC, history or
// whole-database coverage.
type DatasetProvenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
	Complete    bool   `json:"complete"`
}

// DatasetError retains private causes without echoing SQL, bindings or driver diagnostics.
type DatasetError struct {
	Source string
	cause  error
}

func (e *DatasetError) Error() string { return "Oracle dataguard fact read failed (" + e.Source + ")" }

func (e *DatasetError) Unwrap() error { return e.cause }

type datasetSelection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance DatasetProvenance
}

// New fact projections admit the independently reviewed numeric release families.
// Older compatibility SQL remains available; branding does not admit numeric26.
func datasetMajor(major int) error {
	switch major {
	case 19, 21, 23:
		return nil
	}
	return ErrUnsupportedVersion
}

func validDatasetReader(reader Reader) bool {
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

func datasetReadError(ctx context.Context, p datasetSelection, err error) error {
	if err = errors.Join(err, ctx.Err()); err != nil {
		return &DatasetError{p.provenance.Source, err}
	}
	return nil
}

func datasetColumns(result session.Result, p datasetSelection, integerFields ...string) (map[string]int, error) {
	index, ok := projection.TypedColumns(result, p.schema)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, name := range integerFields {
			v := row[index[name]]
			if !v.Null && !numtext.IsUnsignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

// Only shared rowset owns cursor consumption. Domain validation precedes encoding.
func readDatasetJSON(ctx context.Context, q rowset.Queryer, p datasetSelection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &DatasetError{p.provenance.Source, cause}
	}, "limits", "header")
}

// Filter selects reporting INST_IDs; visibility does not prove RAC membership.
type Filter struct{ Instances []int64 }

// Request supplies an observed numeric version and caller-owned SQL reference.
type Request struct {
	OracleVersion string
	Filter        Filter
	Reference     reference.Reference
}

// Reader returns complete results including terminal, close and budget failures.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance describes the selected source, not an atomic cross-query snapshot.
type Provenance struct {
	Source          string  `json:"source"`
	OracleVersion   string  `json:"oracle_version"`
	Scope           string  `json:"scope"`
	Instances       []int64 `json:"instances,omitempty"`
	ResetlogsChange string  `json:"resetlogs_change,omitempty"`
	ResetlogsTime   string  `json:"resetlogs_time,omitempty"`
	Complete        bool    `json:"complete"`
}

var (
	ErrUnsupportedVersion = errors.New("unsupported Oracle Data Guard source version")
	ErrInvalidRequest     = errors.New("invalid Oracle Data Guard request")
	ErrInvalidEvidence    = errors.New("invalid Oracle Data Guard evidence")
)

// Error keeps driver diagnostics private while retaining the original error chain.
type Error struct {
	Source string
	cause  error
}

func (e *Error) Error() string { return "Data Guard read failed (" + e.Source + ")" }

func (e *Error) Unwrap() error { return e.cause }

type field struct {
	name, expression     string
	kind                 rowset.Kind
	exact, date, decimal bool
}

func text(name, expression string) field {
	return field{name: name, expression: expression, kind: rowset.Text}
}

func integer(name, expression string) field {
	return field{name: name, expression: expression, kind: rowset.Integer}
}

func exact(name, expression string) field {
	return field{name: name, expression: "TO_CHAR(" + expression + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')", kind: rowset.Text, exact: true}
}

func date(name, expression string) field {
	return field{name: name, expression: datetime.GregorianDate(expression), kind: rowset.Text, date: true}
}

var decimalText = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[Ee][+-]?[0-9]+)?$`)

func numeric(name, expression string) field {
	f := exact(name, expression)
	f.exact = false
	f.decimal = true
	return f
}

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	fields     []field
	provenance Provenance
	single     bool
}

// Oracle 26ai documentation applies to observed numeric23.26.x; its brand
// does not admit a future numeric26.x source.
func reviewedVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return false
		}
	}
	switch parts[0] {
	case "19", "21", "23":
		return true
	}
	return false
}

func instanceClause(ids []int64) (string, []any, error) {
	var marks []string
	var args []any
	seen := map[int64]bool{}
	for _, id := range ids {
		if id < 1 || seen[id] {
			return "", nil, ErrInvalidRequest
		}
		seen[id] = true
		args = append(args, id)
		marks = append(marks, ":"+strconv.Itoa(len(args)))
	}
	if len(marks) == 0 {
		return "", nil, nil
	}
	return "v.INST_ID IN (" + strings.Join(marks, ", ") + ")", args, nil
}

func selectView(version string, filter Filter, source string, fields []field, where, order string) (selection, error) {
	if !reviewedVersion(version) {
		return selection{}, ErrUnsupportedVersion
	}
	return selectAdmittedView(version, filter, source, fields, where, order)
}

func selectAdmittedView(version string, filter Filter, source string, fields []field, where, order string) (selection, error) {
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	p := selection{args: args, fields: fields, provenance: Provenance{Source: source, OracleVersion: version, Scope: "qualified_visible_reporters", Instances: append([]int64(nil), filter.Instances...)}}
	var expressions []string
	for _, f := range fields {
		p.schema = append(p.schema, rowset.Column{Name: f.name, Kind: f.kind})
		expressions = append(expressions, f.expression+" AS "+f.name)
	}
	if clause != "" {
		if where != "" {
			where += " AND "
		}
		where += clause
	}
	p.sql = "SELECT " + strings.Join(expressions, ", ") + " FROM " + source + " v"
	if where != "" {
		p.sql += " WHERE " + where
	}
	if order != "" {
		p.sql += " ORDER BY " + order
	}
	return p, nil
}

func columns(r session.Result, p selection) (map[string]int, error) {
	idx, ok := projection.TypedColumns(r, p.schema)
	if !ok || (p.single && len(r.Rows) > 1) {
		return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	for _, row := range r.Rows {
		for _, f := range p.fields {
			v := row[idx[f.name]]
			if v.Null {
				continue
			}
			if (f.exact && !numtext.IsUnsignedInteger(v.Text)) || (f.decimal && !decimalText.MatchString(v.Text)) {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			if f.kind == rowset.Integer {
				n, e := strconv.ParseInt(v.Text, 10, 64)
				if e != nil || n < 0 {
					return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
				}
			}
			if f.date {
				d, e := time.Parse("2006-01-02T15:04:05", v.Text)
				if e != nil || d.Year() < 1 || d.Format("2006-01-02T15:04:05") != v.Text {
					return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
				}
			}
		}
		if i, exists := idx["INST_ID"]; exists {
			v := row[i]
			id, e := strconv.ParseInt(v.Text, 10, 64)
			if v.Null || e != nil || id < 1 {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			if len(p.provenance.Instances) > 0 {
				admitted := false
				for _, want := range p.provenance.Instances {
					admitted = admitted || want == id
				}
				if !admitted {
					return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
				}
			}
		}
		if p.provenance.ResetlogsChange != "" {
			value := func(name string) session.Value { return row[idx[name]] }
			if v := value("RESETLOGS_CHANGE"); v.Null || v.Text != p.provenance.ResetlogsChange {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			if v := value("RESETLOGS_TIME"); v.Null || v.Text != p.provenance.ResetlogsTime {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			if v := value("REGISTRAR"); v.Null || v.Text != "RFS" {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			kind := value("KIND")
			applied := value("APPLIED")
			if kind.Null || (kind.Text != "RECEIVED" && kind.Text != "APPLIED") || (kind.Text == "APPLIED" && (applied.Null || applied.Text != "YES")) {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			for _, name := range []string{"THREAD_NUMBER", "SEQUENCE_NUMBER"} {
				if value(name).Null {
					return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
				}
			}
		}
	}
	return idx, nil
}

func query(ctx context.Context, reader Reader, ref reference.Reference, p selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{p.provenance.Source, err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: p.sql, Args: p.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{p.provenance.Source, err}
	}
	return result, nil
}

func open(ctx context.Context, q rowset.Queryer, p selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits) (json.RawMessage, error) {
	p.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate: projection.ValidateRowset(&Error{p.provenance.Source, ErrInvalidEvidence}, func(result session.Result) error {
			_, err := columns(result, p)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "query" || stage == "consume" || stage == "encode" && !errors.Is(cause, rowset.ErrCapacity) {
			return &Error{p.provenance.Source, cause}
		}
		return cause
	})
}

// ScopeOptions carries the source selectors for the options-based fact APIs.
type ScopeOptions struct {
	OracleVersion string
	Filter        Filter
}

// RecoveryIdentityRequest supplies the observed release, caller reference and
// buffered bounds for database incarnation and promotion evidence.
type RecoveryIdentityRequest struct {
	OracleVersion string
	Reference     reference.Reference
	Limits        rowset.Limits
}

func appendRecoveryContainer(fields []field) []field {
	return append(fields, integer("CON_ID", "v.CON_ID"),
		integer("SOURCE_CON_ID", "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))"))
}

func recoveryOptional(row []session.Value, index map[string]int, name string) *session.Value {
	i, ok := index[name]
	if !ok {
		return nil
	}
	value := row[i]
	return &value
}

func readRecoveryIdentity(ctx context.Context, reader Reader, request RecoveryIdentityRequest, p selection) (session.Result, error) {
	if !validDatasetReader(reader) {
		return session.Result{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return session.Result{}, err
	}
	result, err := query(ctx, reader, request.Reference, p)
	if err != nil {
		return session.Result{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return session.Result{}, &Error{p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err())}
	}
	return result, nil
}
