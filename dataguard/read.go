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

// Filter selects reporting INST_IDs; visibility does not prove RAC membership.
type Filter struct{ Instances []int64 }

// Request supplies an observed numeric version and caller-owned SQL reference.
type Request struct {
	OracleVersion string
	Filter        Filter
	Reference     reference.Reference
}

// DatabaseContextRequest admits a database singleton separately from GV reporter rows.
type DatabaseContextRequest struct {
	OracleVersion string
	Reference     reference.Reference
}

// ArchiveProgressOptions admits an incarnation, without a historical date cutoff.
// This is archived-log registration, not the active receive/apply position.
type ArchiveProgressOptions struct {
	Instances       []int64
	ResetlogsChange string
	ResetlogsTime   string
}

// ArchiveProgressRequest supplies incarnation admission and a caller reference.
type ArchiveProgressRequest struct {
	OracleVersion string
	Filter        ArchiveProgressOptions
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
	payload, err := rowset.QueryEnvelopeJSON(ctx, func(ctx context.Context) (rowset.Cursor, error) {
		return open(ctx, q, p)
	}, p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.provenance,
		Validate: projection.ValidateRowset(&Error{p.provenance.Source, ErrInvalidEvidence}, func(result session.Result) error {
			_, err := columns(result, p)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		if stage == "consume" || stage == "encode" && !errors.Is(cause, rowset.ErrCapacity) {
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

// ArchiveDestinationsStatementWithOptions selects the same source as ArchiveDestinationsStatement using one options value.
func ArchiveDestinationsStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveDestinationsStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveDestinationsWithOptions selects the same source as DecodeArchiveDestinations using one options value.
func DecodeArchiveDestinationsWithOptions(result session.Result, options ScopeOptions) (ArchiveDestinationsResult, error) {
	return DecodeArchiveDestinations(result, options.OracleVersion, options.Filter)
}

// ArchiveDestinationsRowsWithOptions selects the same source as ArchiveDestinationsRows using one options value.
func ArchiveDestinationsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveDestinationsRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveDestinationsJSONWithOptions selects the same source as ArchiveDestinationsJSON using one options value.
func ArchiveDestinationsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveDestinationsJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// ArchiveDestinationStatusStatementWithOptions selects the same source as ArchiveDestinationStatusStatement using one options value.
func ArchiveDestinationStatusStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveDestinationStatusStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveDestinationStatusWithOptions selects the same source as DecodeArchiveDestinationStatus using one options value.
func DecodeArchiveDestinationStatusWithOptions(result session.Result, options ScopeOptions) (ArchiveDestinationStatusResult, error) {
	return DecodeArchiveDestinationStatus(result, options.OracleVersion, options.Filter)
}

// ArchiveDestinationStatusRowsWithOptions selects the same source as ArchiveDestinationStatusRows using one options value.
func ArchiveDestinationStatusRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveDestinationStatusRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveDestinationStatusJSONWithOptions selects the same source as ArchiveDestinationStatusJSON using one options value.
func ArchiveDestinationStatusJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveDestinationStatusJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// ArchiveGapsStatementWithOptions selects the same source as ArchiveGapsStatement using one options value.
func ArchiveGapsStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ArchiveGapsStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveGapsWithOptions selects the same source as DecodeArchiveGaps using one options value.
func DecodeArchiveGapsWithOptions(result session.Result, options ScopeOptions) (ArchiveGapsResult, error) {
	return DecodeArchiveGaps(result, options.OracleVersion, options.Filter)
}

// ArchiveGapsRowsWithOptions selects the same source as ArchiveGapsRows using one options value.
func ArchiveGapsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ArchiveGapsRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveGapsJSONWithOptions selects the same source as ArchiveGapsJSON using one options value.
func ArchiveGapsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveGapsJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// ProcessesStatementWithOptions selects the same source as ProcessesStatement using one options value.
func ProcessesStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return ProcessesStatement(options.OracleVersion, options.Filter)
}

// DecodeProcessesWithOptions selects the same source as DecodeProcesses using one options value.
func DecodeProcessesWithOptions(result session.Result, options ScopeOptions) (ProcessesResult, error) {
	return DecodeProcesses(result, options.OracleVersion, options.Filter)
}

// ProcessesRowsWithOptions selects the same source as ProcessesRows using one options value.
func ProcessesRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return ProcessesRows(ctx, q, options.OracleVersion, options.Filter)
}

// ProcessesJSONWithOptions selects the same source as ProcessesJSON using one options value.
func ProcessesJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ProcessesJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// RecoveryProgressStatementWithOptions selects the same source as RecoveryProgressStatement using one options value.
func RecoveryProgressStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return RecoveryProgressStatement(options.OracleVersion, options.Filter)
}

// DecodeRecoveryProgressWithOptions selects the same source as DecodeRecoveryProgress using one options value.
func DecodeRecoveryProgressWithOptions(result session.Result, options ScopeOptions) (RecoveryProgressResult, error) {
	return DecodeRecoveryProgress(result, options.OracleVersion, options.Filter)
}

// RecoveryProgressRowsWithOptions selects the same source as RecoveryProgressRows using one options value.
func RecoveryProgressRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return RecoveryProgressRows(ctx, q, options.OracleVersion, options.Filter)
}

// RecoveryProgressJSONWithOptions selects the same source as RecoveryProgressJSON using one options value.
func RecoveryProgressJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return RecoveryProgressJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// StandbyLogsStatementWithOptions selects the same source as StandbyLogsStatement using one options value.
func StandbyLogsStatementWithOptions(options ScopeOptions) (string, []any, error) {
	return StandbyLogsStatement(options.OracleVersion, options.Filter)
}

// DecodeStandbyLogsWithOptions selects the same source as DecodeStandbyLogs using one options value.
func DecodeStandbyLogsWithOptions(result session.Result, options ScopeOptions) (StandbyLogsResult, error) {
	return DecodeStandbyLogs(result, options.OracleVersion, options.Filter)
}

// StandbyLogsRowsWithOptions selects the same source as StandbyLogsRows using one options value.
func StandbyLogsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions) (*sql.Rows, error) {
	return StandbyLogsRows(ctx, q, options.OracleVersion, options.Filter)
}

// StandbyLogsJSONWithOptions selects the same source as StandbyLogsJSON using one options value.
func StandbyLogsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ScopeOptions, limits rowset.Limits) (json.RawMessage, error) {
	return StandbyLogsJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}

// ArchiveProgressSourceOptions carries the source selectors for the options-based fact APIs.
type ArchiveProgressSourceOptions struct {
	OracleVersion string
	Filter        ArchiveProgressOptions
}

// ArchiveProgressStatementWithOptions selects the same source as ArchiveProgressStatement using one options value.
func ArchiveProgressStatementWithOptions(options ArchiveProgressSourceOptions) (string, []any, error) {
	return ArchiveProgressStatement(options.OracleVersion, options.Filter)
}

// DecodeArchiveProgressWithOptions selects the same source as DecodeArchiveProgress using one options value.
func DecodeArchiveProgressWithOptions(result session.Result, options ArchiveProgressSourceOptions) (ArchiveProgressResult, error) {
	return DecodeArchiveProgress(result, options.OracleVersion, options.Filter)
}

// ArchiveProgressRowsWithOptions selects the same source as ArchiveProgressRows using one options value.
func ArchiveProgressRowsWithOptions(ctx context.Context, q rowset.Queryer, options ArchiveProgressSourceOptions) (*sql.Rows, error) {
	return ArchiveProgressRows(ctx, q, options.OracleVersion, options.Filter)
}

// ArchiveProgressJSONWithOptions selects the same source as ArchiveProgressJSON using one options value.
func ArchiveProgressJSONWithOptions(ctx context.Context, q rowset.Queryer, options ArchiveProgressSourceOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ArchiveProgressJSON(ctx, q, options.OracleVersion, options.Filter, limits)
}
