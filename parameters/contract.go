// Package parameters selects read-only effective instance and stored SPFILE
// parameter rows in an already-admitted current container.
package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete bounded results, retaining terminal/close failures.
// A session-owned reader keeps its own lease, cancellation and transport policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits the known source version, filters and caller-owned SQL reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("parameter reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed projections, exact values or selections.
	ErrInvalidEvidence = errors.New("invalid Oracle parameter evidence")
)

func queryParameters(ctx context.Context, reader Reader, ref reference.Reference, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return result, nil
}

func optional(row []session.Value, index map[string]int, name string) *session.Value {
	i, present := index[name]
	if !present {
		return nil
	}
	value := row[i]
	return &value
}

func parameterColumns(result session.Result, plan selection) (map[string]int, error) {
	fail := func() (map[string]int, error) {
		return nil, &Error{Source: plan.provenance.Source, cause: ErrInvalidEvidence}
	}
	names := make([]string, len(plan.schema))
	for i, column := range plan.schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return fail()
	}
	for _, row := range result.Rows {
		for _, column := range plan.schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return fail()
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return fail()
				}
			}
		}
		name := row[index["NAME"]]
		if plan.provenance.NameClass != "" && (name.Null || !strings.HasPrefix(name.Text, "_")) {
			return fail()
		}
		if !strings.Contains(strings.ToLower(name.Text), strings.ToLower(plan.provenance.NameContains)) {
			return fail()
		}
		if !admittedText(name, plan.provenance.Names) {
			return fail()
		}
		if len(plan.provenance.Instances) > 0 {
			id := row[index["INST_ID"]]
			number, err := strconv.ParseInt(id.Text, 10, 64)
			found := false
			for _, wanted := range plan.provenance.Instances {
				found = found || wanted == number
			}
			if id.Null || err != nil || !found {
				return fail()
			}
		}
		if len(plan.provenance.SIDs) > 0 && !admittedText(row[index["SID"]], plan.provenance.SIDs) {
			return fail()
		}
	}
	return index, nil
}

func admittedText(value session.Value, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	if value.Null {
		return false
	}
	for _, wanted := range allowed {
		if value.Text == wanted {
			return true
		}
	}
	return false
}

// JSONEnvelope keeps source/filter provenance separate from ordered native data.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Error avoids formatting driver errors, SQL or bind data. Unwrap retains the
// cause for errors.Is/As and deliberately private diagnostics.
type Error struct {
	Source string
	cause  error
}

// Error returns only the static source, never SQL, bindings or driver text.
func (e *Error) Error() string { return "parameter read failed (" + e.Source + ")" }

// Unwrap retains reported causes for explicit private inspection.
func (e *Error) Unwrap() error { return e.cause }

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{Source: plan.provenance.Source, cause: err}
	}
	return rows, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, plan)
	}, plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(&Error{Source: plan.provenance.Source, cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := parameterColumns(result, plan)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{Source: plan.provenance.Source, cause: cause}
	}, "limits", "query", "validate")
}

// SourceOptions carries the source selectors for the options-based fact APIs.
type SourceOptions struct {
	OracleMajor int
	Filter      Filter
}

// Filter selects exact parameter names and reporting instance numbers. Empty
// slices select all visible values, not proven complete RAC membership.
type Filter struct {
	Names     []string
	Instances []int64
}

// MaxFilterValues includes an automatically retained SPFILE wildcard in its IN budget.
const MaxFilterValues = 1000

var (
	// ErrInvalidFilter rejects invalid strings/IDs or an Oracle IN-list budget overflow.
	ErrInvalidFilter = errors.New("invalid parameter filter")
	// ErrUnsupportedVersion rejects a major below the reviewed11g source floor.
	ErrUnsupportedVersion = errors.New("unsupported Oracle parameter source version")
)

// Provenance describes selected dynamic-view or explicit fixed-table facts. It is not
// evidence of complete physical SPFILE contents, RAC membership or inheritance.
type Provenance struct {
	Source       string   `json:"source"`
	OracleMajor  int      `json:"oracle_major"`
	Scope        string   `json:"scope"`
	Names        []string `json:"names,omitempty"`
	Instances    []int64  `json:"instances,omitempty"`
	SIDs         []string `json:"sids,omitempty"`
	SIDPolicy    string   `json:"sid_policy,omitempty"`
	NameClass    string   `json:"name_class,omitempty"`
	NameContains string   `json:"name_contains,omitempty"`
}

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	provenance Provenance
}

func source(view string, major int, names []string, instances []int64) Provenance {
	scope := "non_cdb_visible_instance_rows"
	if major >= 12 {
		scope = "current_container_and_whole_database_rows"
	}
	return Provenance{Source: view, OracleMajor: major, Scope: scope, Names: append([]string(nil), names...), Instances: append([]int64(nil), instances...)}
}

func validate(names []string, instances []int64, sids []string) error {
	if len(names) > MaxFilterValues || len(instances) > MaxFilterValues || len(sids) > MaxFilterValues {
		return ErrInvalidFilter
	}
	for _, values := range [][]string{names, sids} {
		for _, value := range values {
			if value == "" || len(value) > 80 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return ErrInvalidFilter
			}
		}
	}
	for _, id := range instances {
		if id < 1 {
			return ErrInvalidFilter
		}
	}
	return nil
}

func build(major int, provenance Provenance, columns []rowset.Column, sids []string) selection {
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "p." + column.Name + " AS " + column.Name
	}
	clauses := []string{}
	// Prefix admission must not inherit a caller's linguistic collation.
	if provenance.NameClass == "leading_underscore_only" {
		clauses = append(clauses, `NLSSORT(SUBSTR(p.NAME,1,1), 'NLS_SORT=BINARY') = NLSSORT('_', 'NLS_SORT=BINARY')`)
	}
	args := make([]any, 0, len(provenance.Names)+len(provenance.Instances)+len(sids))
	if major >= 12 {
		clauses = append(clauses, "p.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))")
	}
	if len(provenance.Instances) > 0 {
		binds := make([]string, len(provenance.Instances))
		for i, id := range provenance.Instances {
			args = append(args, id)
			binds[i] = ":" + strconv.Itoa(len(args))
		}
		clauses = append(clauses, "p.INST_ID IN ("+strings.Join(binds, ",")+")")
	}
	for _, filter := range []struct {
		name   string
		values []string
	}{{"NAME", provenance.Names}, {"SID", sids}} {
		if len(filter.values) == 0 {
			continue
		}
		binds := make([]string, len(filter.values))
		for i, value := range filter.values {
			args = append(args, value)
			binds[i] = "NLSSORT(:" + strconv.Itoa(len(args)) + ", 'NLS_SORT=BINARY')"
		}
		clauses = append(clauses, "NLSSORT(p."+filter.name+", 'NLS_SORT=BINARY') IN ("+strings.Join(binds, ",")+")")
	}
	text := "SELECT " + strings.Join(aliases, ", ") + " FROM " + provenance.Source + " p"
	if len(clauses) > 0 {
		text += " WHERE " + strings.Join(clauses, " AND ")
	}
	text += " ORDER BY p.INST_ID"
	if major >= 12 {
		text += ", p.CON_ID"
	}
	text += ", NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
	if provenance.Source == "GV$SPPARAMETER" {
		text += ", NLSSORT(p.SID, 'NLS_SORT=BINARY')"
	}
	if provenance.Source != "GV$SYSTEM_PARAMETER" {
		text += ", p.ORDINAL"
	}
	return selection{sql: text, args: args, schema: columns, provenance: provenance}
}
