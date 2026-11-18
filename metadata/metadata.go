// Package metadata captures separately selected Oracle DDL as evidence only.
// It executes no captured DDL and changes no DBMS_METADATA session transforms.
package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Kind restricts capture to the explicitly reviewed metadata object types.
type Kind string

const (
	// Tablespace captures one tablespace definition.
	Tablespace Kind = "TABLESPACE"
	// Directory captures one directory definition, without filesystem access.
	Directory Kind = "DIRECTORY"
	// Trigger captures one trigger definition, without firing or changing it.
	Trigger Kind = "TRIGGER"
)

// Options admits one named object and a finite complete UTF-8 DDL byte budget.
// MaxDDLBytes zero defaults to 8MiB; OracleMajor must be at least 11.
// Owner is required for triggers; it may be empty for non-schema objects.
type Options struct {
	OracleMajor int
	Kind        Kind
	Name        string
	Owner       string
	MaxDDLBytes int
}

// Reader supplies complete, bounded results with terminal/close errors retained.
// Its owner admits DBMS_METADATA privileges and configures complete CLOB transport.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies the selected object, a caller-owned reference and row/byte
// budgets. Reader transport budgets must also admit the complete CLOB.
type Request struct {
	Options   Options
	Reference reference.Reference
	Limits    rowset.Limits
}

// Provenance explicitly records that inherited caller transforms were not changed
// or inspected. Output is captured evidence, not replay or compatibility approval.
type Provenance struct {
	Source      string `json:"source"`
	OracleMajor int    `json:"oracle_major"`
	Scope       string `json:"scope"`
	Transforms  string `json:"transforms"`
}

// Record retains exact DDL and the Oracle CLOB length in UCS2 code units. SQL NULL
// differs from omitted owner; missing/truncated DDL never becomes complete evidence.
type Record struct {
	ObjectType    session.Value
	Name          session.Value
	Owner         session.Value
	DDL           session.Value
	CLOBCodeUnits session.Value
}

// Result contains exactly one completely captured object, without UI formatting.
type Result struct {
	Provenance Provenance
	Records    []Record
	Complete   bool
}

var (
	// ErrInvalidRequest rejects unsupported types, names, references or budgets.
	ErrInvalidRequest = errors.New("invalid Oracle metadata request")
	// ErrUnsupportedVersion rejects releases before Oracle 11g.
	ErrUnsupportedVersion = errors.New("Oracle metadata requires Oracle 11g or later")
	// ErrInvalidEvidence rejects malformed or incomplete DDL/length observations.
	ErrInvalidEvidence = errors.New("invalid or incomplete Oracle DDL evidence")
	// ErrNoRows reports missing metadata instead of an authoritative empty capture.
	ErrNoRows = errors.New("selected Oracle metadata returned no rows")
)

// Error exposes a static operation and retains private causes for errors.Is/As.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "Oracle metadata " + e.Operation + " failed" }
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql        string
	args       []any
	options    Options
	provenance Provenance
}

var schema = []rowset.Column{{Name: "OBJECT_TYPE", Kind: rowset.Text}, {Name: "OBJECT_NAME", Kind: rowset.Text}, {Name: "OWNER", Kind: rowset.Text}, {Name: "DDL", Kind: rowset.Text}, {Name: "CLOB_CODE_UNITS", Kind: rowset.Text}}

// Statement returns one explicitly selected capture query with a native CLOB. It
// uses no SUBSTR, VARCHAR cast, persistent transform change or DDL execution.
func Statement(options Options) (string, []any, error) {
	p, e := selectMetadata(options)
	return p.sql, p.args, e
}
func selectMetadata(options Options) (selection, error) {
	if options.OracleMajor < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	switch options.Kind {
	case Tablespace, Directory, Trigger:
	default:
		return selection{}, ErrInvalidRequest
	}
	for _, name := range []string{options.Name, options.Owner} {
		if len(name) > 128 || !utf8.ValidString(name) || strings.ContainsRune(name, 0) {
			return selection{}, ErrInvalidRequest
		}
	}
	if options.Name == "" || (options.Kind == Trigger && options.Owner == "") || options.MaxDDLBytes < 0 {
		return selection{}, ErrInvalidRequest
	}
	if options.MaxDDLBytes == 0 {
		options.MaxDDLBytes = 8 << 20
	}
	var owner any
	if options.Owner != "" {
		owner = options.Owner
	}
	// The type is a fixed allowlisted literal. Names/owners are bound, never
	// interpolated identifiers. GET_DDL inherits the caller's existing transforms.
	query := `WITH requested AS (SELECT CAST(:1 AS VARCHAR2(128)) AS OBJECT_NAME, CAST(:2 AS VARCHAR2(128)) AS OWNER FROM DUAL),
 definition AS (SELECT r.OBJECT_NAME, r.OWNER, DBMS_METADATA.GET_DDL('` + string(options.Kind) + `',r.OBJECT_NAME,r.OWNER) AS DDL FROM requested r)
 SELECT '` + string(options.Kind) + `' AS OBJECT_TYPE, d.OBJECT_NAME AS OBJECT_NAME,
 d.OWNER AS OWNER, d.DDL AS DDL,
 TO_CHAR(DBMS_LOB.GETLENGTH(d.DDL), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CLOB_CODE_UNITS FROM definition d`
	return selection{sql: query, args: []any{options.Name, owner}, options: options, provenance: Provenance{Source: "DBMS_METADATA.GET_DDL+DBMS_LOB.GETLENGTH", OracleMajor: options.OracleMajor, Scope: "explicit_object_connected_container", Transforms: "caller_session_inherited_unmodified"}}, nil
}

// Decode validates exact identity, CLOB length and byte budget before publishing.
// A pure decoder cannot prove EOF; use Read with a complete admitted Reader.
func Decode(result session.Result, options Options) (Result, error) {
	p, e := selectMetadata(options)
	if e != nil {
		return Result{}, e
	}
	return decode(result, p)
}
func decode(result session.Result, p selection) (Result, error) {
	names := []string{"OBJECT_TYPE", "OBJECT_NAME", "OWNER", "DDL", "CLOB_CODE_UNITS"}
	index, ok := projection.Columns(result, names...)
	if !ok || len(result.Rows) > 1 {
		return Result{}, ErrInvalidEvidence
	}
	if len(result.Rows) == 0 {
		return Result{}, ErrNoRows
	}
	row := result.Rows[0]
	for _, v := range row {
		if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
			return Result{}, ErrInvalidEvidence
		}
	}
	r := Record{ObjectType: row[index["OBJECT_TYPE"]], Name: row[index["OBJECT_NAME"]], Owner: row[index["OWNER"]], DDL: row[index["DDL"]], CLOBCodeUnits: row[index["CLOB_CODE_UNITS"]]}
	if r.ObjectType.Null || r.ObjectType.Text != string(p.options.Kind) || r.Name.Null || r.Name.Text != p.options.Name || (p.options.Owner == "" && !r.Owner.Null) || (p.options.Owner != "" && (r.Owner.Null || r.Owner.Text != p.options.Owner)) || r.DDL.Null || r.CLOBCodeUnits.Null {
		return Result{}, ErrInvalidEvidence
	}
	if len(r.DDL.Text) > p.options.MaxDDLBytes {
		return Result{}, rowset.ErrCapacity
	}
	expected, err := strconv.ParseInt(r.CLOBCodeUnits.Text, 10, 64)
	if err != nil || expected < 1 {
		return Result{}, ErrInvalidEvidence
	}
	// Oracle CLOB length APIs use UCS2 codepoint semantics. A supplementary
	// Unicode scalar consumes two units; rune count alone would reject valid DDL.
	actual := int64(0)
	for _, r := range r.DDL.Text {
		actual++
		if r > 0xffff {
			actual++
		}
	}
	if actual != expected {
		return Result{}, ErrInvalidEvidence
	}
	return Result{Provenance: p.provenance, Records: []Record{r}, Complete: true}, nil
}

// Read captures one complete CLOB through an admitted reader and publishes no
// record on cancellation, terminal/close errors, missing rows or any budget error.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	p, err := selectMetadata(request.Options)
	if err != nil {
		return Result{}, err
	}
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return Result{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return Result{}, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return Result{}, &Error{Operation: "query", cause: err}
	}
	if len(result.Rows) > limits.MaxRows {
		return Result{}, rowset.ErrCapacity
	}
	retained := 0
	for _, row := range result.Rows {
		for _, v := range row {
			if len(v.Text) > limits.MaxBytes-retained {
				return Result{}, rowset.ErrCapacity
			}
			retained += len(v.Text)
		}
	}
	facts, de := decode(result, p)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return Result{}, &Error{Operation: "decode", cause: err}
	}
	return facts, nil
}

// Rows returns the actual caller-native CLOB cursor. Its owner must consume the
// full value, verify CLOB_CODE_UNITS, enforce budgets and check Err/Close. This raw
// method does not certify completion, allocate an owned lease or truncate text.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, e := selectMetadata(options)
	if e != nil {
		return nil, e
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// JSON owns a separate cursor and uses shared rowset finalization before checking
// the complete CLOB/identity and publishing bounded standard JSON. Native drivers
// must supply complete CLOBs as string/bytes; unsupported values fail explicitly.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := selectMetadata(options)
	if err != nil {
		return nil, err
	}
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{Operation: "query", cause: err}
	}
	table, err := rowset.Read(ctx, rows, schema, resolved)
	if err != nil {
		return nil, &Error{Operation: "consume", cause: err}
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	facts, err := decode(result, p)
	if err != nil {
		return nil, &Error{Operation: "decode", cause: err}
	}
	payload, err := json.Marshal(facts)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{Operation: "encode", cause: err}
	}
	return json.RawMessage(payload), nil
}
