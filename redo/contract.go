// Package redo exposes reviewed read-only redo facts over admitted caller handles.
package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete finite-budget results, including terminal and close
// failures. Its caller retains admission, session lifetime and cancellation.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Provenance identifies the admitted view and scope. Complete is true only after
// an owned reader or JSON cursor completes successfully. Visible instance rows
// never establish complete physical RAC membership or a common snapshot.
type Provenance struct {
	Source          string   `json:"source"`
	OracleMajor     int      `json:"oracle_major"`
	Scope           string   `json:"scope"`
	Instances       []int64  `json:"instances,omitempty"`
	Complete        bool     `json:"complete"`
	From            string   `json:"from_local,omitempty"`
	Until           string   `json:"until_local,omitempty"`
	OptionalColumns []string `json:"optional_columns,omitempty"`
}

var (
	// ErrUnsupportedVersion rejects unsupported source releases or columns.
	ErrUnsupportedVersion = errors.New("unsupported Oracle redo source version")
	// ErrInvalidRequest rejects invalid filters, readers and SQL references.
	ErrInvalidRequest = errors.New("invalid Oracle redo request")
	// ErrInvalidEvidence rejects malformed shapes, exact integers or selection.
	ErrInvalidEvidence = errors.New("invalid Oracle redo evidence")
)

// Error exposes only a static source. Unwrap retains causes for errors.Is/As and
// deliberately private diagnostics; driver errors can contain sensitive inputs.
type Error struct {
	Source string
	cause  error
}

// Error returns a static source without formatting driver details.
func (e *Error) Error() string { return "redo read failed (" + e.Source + ")" }

// Unwrap preserves the original cause for deliberate inspection.
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql         string
	args        []any
	schema      []rowset.Column
	integerText []string
	provenance  Provenance
}

func columns(result session.Result, plan selection) (map[string]int, error) {
	names := make([]string, len(plan.schema))
	for i, c := range plan.schema {
		names[i] = c.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
	}
	integers := map[string]bool{}
	for _, c := range plan.schema {
		if c.Kind == rowset.Integer {
			integers[c.Name] = true
		}
	}
	for _, name := range plan.integerText {
		integers[name] = true
	}
	for _, row := range result.Rows {
		for _, name := range []string{"INST_ID", "SOURCE_INST_ID"} {
			pos, exists := index[name]
			if !exists {
				continue
			}

			v := row[pos]
			id, err := strconv.ParseInt(v.Text, 10, 64)
			if v.Null || err != nil || id <= 0 {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
		}

		for _, c := range plan.schema {
			v := row[index[c.Name]]
			if !v.Null && (c.Name == "FIRST_TIME" || c.Name == "NEXT_TIME" || c.Name == "RESETLOGS_TIME" || c.Name == "COMPLETION_TIME") {
				parsed, err := time.Parse("2006-01-02T15:04:05", v.Text)
				if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02T15:04:05") != v.Text {
					return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
				}
			}

			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
			if !v.Null && integers[c.Name] {
				// SCNs can exceed int64. Preserve exact nonnegative Oracle NUMBER text.
				if !numeric.IsUnsignedInteger(v.Text) {
					return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
				}
			}
		}
		if pos, exists := index["INST_ID"]; exists && len(plan.provenance.Instances) > 0 {
			v := row[pos]
			id, err := strconv.ParseInt(v.Text, 10, 64)
			admitted := false
			for _, selected := range plan.provenance.Instances {
				admitted = admitted || selected == id
			}
			if err != nil || v.Null || !admitted {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
		}
	}
	return index, nil
}

func optional(row []session.Value, index map[string]int, name string) *session.Value {
	if pos, ok := index[name]; ok {
		v := row[pos]
		return &v
	}
	return nil
}

func query(ctx context.Context, reader Reader, ref reference.Reference, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{plan.provenance.Source, err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{plan.provenance.Source, err}
	}
	return result, nil
}

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql, plan.args...)
	if err != nil {
		return nil, &Error{plan.provenance.Source, err}
	}
	return rows, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits, validate func(session.Result) error) (json.RawMessage, error) {
	plan.provenance.Complete = true
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, plan)
	}, plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{plan.provenance.Source, cause}
	}, "limits", "query")
}

func instanceClause(ids []int64) (string, []any, error) {
	if len(ids) > 1000 {
		return "", nil, ErrInvalidRequest
	}
	seen := map[int64]bool{}
	args := make([]any, 0, len(ids))
	binds := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return "", nil, ErrInvalidRequest
		}
		seen[id] = true
		args = append(args, id)
		binds = append(binds, ":"+strconv.Itoa(len(args)))
	}
	if len(ids) == 0 {
		return "", args, nil
	}
	return "v.INST_ID IN (" + strings.Join(binds, ",") + ")", args, nil
}

func exactNumber(expr string) string {
	return "TO_CHAR(" + expr + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}

// LogOptions carries the source selectors for the options-based fact APIs.
type LogOptions struct {
	OracleMajor int
	Filter      Filter
}

// HistoryOptions carries the source selectors for the options-based fact APIs.
type HistoryOptions struct {
	OracleMajor int
	Filter      HistoryFilter
}

// Filter selects reporting INST_ID values. Empty selects all visible instances;
// this is not a claim of complete physical RAC membership.
type Filter struct{ Instances []int64 }

// Request supplies admitted version, instance selection and execution reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

// HistoryFilter selects a finite database-local DATE interval [From,Until).
// Timezones are not converted: wall-clock fields are used verbatim, to seconds.
// Bounds must be whole seconds in one location, increasing and at most 31 days.
type HistoryFilter struct {
	Instances   []int64
	From, Until time.Time
}

// HistoryRequest admits an explicit local-wall-clock interval and SQL reference.
type HistoryRequest struct {
	OracleMajor int
	Filter      HistoryFilter
	Reference   reference.Reference
}

func historyBounds(filter HistoryFilter) (string, string, error) {
	if filter.From.Year() < 1 || filter.Until.Year() > 9999 || filter.Until.Year() < 1 || filter.From.Year() > 9999 {
		return "", "", ErrInvalidRequest
	}
	if filter.From.IsZero() || filter.Until.IsZero() || filter.From.Nanosecond() != 0 || filter.Until.Nanosecond() != 0 || filter.From.Location().String() != filter.Until.Location().String() {
		return "", "", ErrInvalidRequest
	}
	// Compare wall clocks, not elapsed seconds, so DST cannot widen the admitted interval.
	layout := "2006-01-02T15:04:05"
	from := filter.From.Format(layout)
	until := filter.Until.Format(layout)
	a, e1 := time.Parse(layout, from)
	b, e2 := time.Parse(layout, until)
	if e1 != nil || e2 != nil || !b.After(a) || b.Sub(a) > 31*24*time.Hour {
		return "", "", ErrInvalidRequest
	}
	return from, until, nil
}
