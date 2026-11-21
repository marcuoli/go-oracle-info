// Package asm exposes reviewed read-only ASM client facts over admitted caller handles.
package asm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
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
	Source      string  `json:"source"`
	OracleMajor int     `json:"oracle_major"`
	Scope       string  `json:"scope"`
	Instances   []int64 `json:"instances,omitempty"`
	Complete    bool    `json:"complete"`
	SourceRole  string  `json:"source_role,omitempty"`
}

var (
	// ErrUnsupportedVersion rejects unsupported source releases or columns.
	ErrUnsupportedVersion = errors.New("unsupported Oracle ASM client source version")
	// ErrInvalidRequest rejects invalid filters, readers and SQL references.
	ErrInvalidRequest = errors.New("invalid Oracle ASM client request")
	// ErrInvalidEvidence rejects malformed shapes, exact integers or selection.
	ErrInvalidEvidence = errors.New("invalid Oracle ASM client evidence")
)

// Error exposes only a static source. Unwrap retains causes for errors.Is/As and
// deliberately private diagnostics; driver errors can contain sensitive inputs.
type Error struct {
	Source string
	cause  error
}

// Error returns a static source without formatting driver details.
func (e *Error) Error() string { return "ASM client read failed (" + e.Source + ")" }

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
			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, &Error{plan.provenance.Source, ErrInvalidEvidence}
			}
			if !v.Null && integers[c.Name] {
				// Preserve exact nonnegative Oracle NUMBER text without float rounding.
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
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql, plan.args...), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate:   projection.ValidateRowset(ErrInvalidEvidence, validate),
	})
	return payload, projection.WrapEnvelopeErrorExcept(err, func(cause error) error {
		return &Error{plan.provenance.Source, cause}
	}, "limits")
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

// SourceRole is caller-observed source context, never inferred from client names.
type SourceRole string

const (
	// DatabaseInstance describes connections from databases with open ASM files.
	DatabaseInstance SourceRole = "database_instance"
	// ASMInstance describes clients using disk groups managed by ASM.
	ASMInstance SourceRole = "asm_instance"
)

// Filter selects reporting IDs and requires explicit source-role admission.
type Filter struct {
	Instances  []int64
	SourceRole SourceRole
}

// Request supplies admitted source version, role, instances and SQL reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

// Client preserves raw source values and SQL NULL. Optional nil fields mean
// an unselected version/capability column, distinct from a selected SQL NULL.
// SourceRole is caller admission, not discovered. Database-instance rows describe ASM connections for open ASM files; ASM-instance rows identify clients. Version text stays raw; lexical min/max is not a numeric version comparison.
type Client struct {
	InstanceID        session.Value
	GroupNumber       session.Value
	InstanceName      session.Value
	DatabaseName      session.Value
	Status            session.Value
	SoftwareVersion   session.Value
	CompatibleVersion session.Value
	ContainerID       *session.Value
}

// ClientsResult preserves complete selected rows and source provenance.
// Pure Decode does not prove transport completeness; Read sets Complete on success.
type ClientsResult struct {
	Provenance Provenance
	Rows       []Client
}

// ClientsStatement selects the reviewed source without SQL row truncation.
func ClientsStatement(major int, filter Filter) (string, []any, error) {
	plan, err := clientsPlan(major, filter)
	return plan.sql, plan.args, err
}

func clientsPlan(major int, filter Filter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer},
		{Name: "GROUP_NUMBER", Kind: rowset.Integer},
		{Name: "INSTANCE_NAME", Kind: rowset.Text},
		{Name: "DB_NAME", Kind: rowset.Text},
		{Name: "STATUS", Kind: rowset.Text},
		{Name: "SOFTWARE_VERSION", Kind: rowset.Text},
		{Name: "COMPATIBLE_VERSION", Kind: rowset.Text},
	}
	projection := "v.INST_ID AS INST_ID" + ", " +
		"v.GROUP_NUMBER AS GROUP_NUMBER" + ", " +
		"v.INSTANCE_NAME AS INSTANCE_NAME" + ", " +
		"v.DB_NAME AS DB_NAME" + ", " +
		"v.STATUS AS STATUS" + ", " +
		"v.SOFTWARE_VERSION AS SOFTWARE_VERSION" + ", " +
		"v.COMPATIBLE_VERSION AS COMPATIBLE_VERSION"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		projection += ", v.CON_ID AS CON_ID"
	}
	plan := selection{schema: schema, provenance: Provenance{Source: "GV$ASM_CLIENT", OracleMajor: major, Scope: "qualified_visible_instances"}}
	clause, args, err := instanceClause(filter.Instances)
	if err != nil {
		return selection{}, err
	}
	plan.args = args
	plan.provenance.Instances = append([]int64(nil), filter.Instances...)
	where := clause
	if filter.SourceRole != DatabaseInstance && filter.SourceRole != ASMInstance {
		return selection{}, ErrInvalidRequest
	}
	plan.provenance.SourceRole = string(filter.SourceRole)
	order := "v.INST_ID, v.GROUP_NUMBER, v.INSTANCE_NAME, v.DB_NAME"
	plan.sql = "SELECT " + projection + " FROM GV$ASM_CLIENT v"
	if where != "" {
		plan.sql += " WHERE " + where
	}
	plan.sql += " ORDER BY " + order
	return plan, nil
}

// DecodeClients validates every supplied row before publishing any typed slice.
func DecodeClients(result session.Result, major int, filter Filter) (ClientsResult, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return ClientsResult{}, err
	}
	return decodeClients(result, plan)
}

func decodeClients(result session.Result, plan selection) (ClientsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return ClientsResult{}, err
	}
	rows := make([]Client, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Client{
			InstanceID:        row[index["INST_ID"]],
			GroupNumber:       row[index["GROUP_NUMBER"]],
			InstanceName:      row[index["INSTANCE_NAME"]],
			DatabaseName:      row[index["DB_NAME"]],
			Status:            row[index["STATUS"]],
			SoftwareVersion:   row[index["SOFTWARE_VERSION"]],
			CompatibleVersion: row[index["COMPATIBLE_VERSION"]],
			ContainerID:       optional(row, index, "CON_ID"),
		}
	}
	return ClientsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadClients executes once through the admitted bounded reader, retaining no
// prefix on query, late decode, close or cancellation failure. It owns no retry.
func ReadClients(ctx context.Context, reader Reader, request Request) (ClientsResult, error) {
	plan, err := clientsPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return ClientsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return ClientsResult{}, err
	}
	facts, decodeErr := decodeClients(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ClientsResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ClientsRows returns the actual native cursor; callers own iteration, Err,
// Close, deadlines and finite stream budgets. There is no implicit SQL limit.
func ClientsRows(ctx context.Context, q rowset.Queryer, major int, filter Filter) (*sql.Rows, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ClientsJSON consumes its own identical cursor through shared rowset lifecycle
// and finite limits. No bytes are returned on any terminal/close/budget failure.
func ClientsJSON(ctx context.Context, q rowset.Queryer, major int, filter Filter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := clientsPlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error { _, err := decodeClients(result, plan); return err })
}

// ClientsOptions carries the source selectors for the options-based fact APIs.
type ClientsOptions struct {
	OracleMajor int
	Filter      Filter
}

// ClientsStatementWithOptions selects the same source as ClientsStatement using one options value.
func ClientsStatementWithOptions(options ClientsOptions) (string, []any, error) {
	return ClientsStatement(options.OracleMajor, options.Filter)
}

// DecodeClientsWithOptions selects the same source as DecodeClients using one options value.
func DecodeClientsWithOptions(result session.Result, options ClientsOptions) (ClientsResult, error) {
	return DecodeClients(result, options.OracleMajor, options.Filter)
}

// ClientsRowsWithOptions selects the same source as ClientsRows using one options value.
func ClientsRowsWithOptions(ctx context.Context, q rowset.Queryer, options ClientsOptions) (*sql.Rows, error) {
	return ClientsRows(ctx, q, options.OracleMajor, options.Filter)
}

// ClientsJSONWithOptions selects the same source as ClientsJSON using one options value.
func ClientsJSONWithOptions(ctx context.Context, q rowset.Queryer, options ClientsOptions, limits rowset.Limits) (json.RawMessage, error) {
	return ClientsJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}
