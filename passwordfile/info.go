// Package passwordfile selects and decodes observed Oracle password-file metadata.
// It reads no file contents, credentials or users and changes no cache or setting.
package passwordfile

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

// Reader returns complete bounded results with terminal/close failures retained.
// Its owner supplies admission, lease lifetime, cancellation and transport policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits source options and a valid caller-owned execution reference.
type Request struct {
	Options   Options
	Reference reference.Reference
}

// State separates an observed row, successful empty source and unavailable reads.
type State string

const (
	Available   State = "available"
	NoRows      State = "no_rows"
	Unavailable State = "unavailable"
	Unsupported State = "unsupported"
)

// Info retains observed metadata only, without format/location/file inference.
// IsExascale nil means unselected; a selected SQL NULL has a nonnil Null value.
// ContainerID retains exact integer text; raw format/flag spelling is preserved.
type Info struct {
	FileName    session.Value
	Format      session.Value
	IsASM       session.Value
	IsExascale  *session.Value
	ContainerID session.Value
}

// Observation reports whether the view returned a row, not whether a password file
// exists or was inspected. NoRows never means a verified absent/empty file.
type Observation struct {
	State      State
	Provenance Provenance
	Value      *Info
}

// Error exposes a static operation while retaining causes for errors.Is/As and
// deliberate private diagnostics. Its message never formats a driver/file value.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "password-file " + e.Operation + " failed" }

func (e *Error) Unwrap() error { return e.cause }

// JSONEnvelope separates domain provenance from the ordered native rowset.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Decode validates an entire source observation, retaining successful empty and
// NULL states. The documented view is singular; extra rows are invalid evidence.
func Decode(result session.Result, options Options) (Observation, error) {
	plan, err := selectMetadata(options)
	if err != nil {
		return Observation{State: Unsupported}, err
	}
	return decode(result, plan)
}

// Read queries once through the admitted owned reader. No failed prefix, retry,
// fallback, cache flush or file/credential access occurs here.
func Read(ctx context.Context, reader Reader, request Request) (Observation, error) {
	plan, err := selectMetadata(request.Options)
	if err != nil {
		return Observation{State: Unsupported}, err
	}
	unavailable := Observation{State: Unavailable, Provenance: plan.provenance}
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return unavailable, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return unavailable, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "query", cause: err}
	}
	facts, decodeErr := decode(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the exact caller-owned native cursor; its owner must iterate,
// inspect Err, close on every path and supply stream deadlines/budgets.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := selectMetadata(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// JSON independently reads/finalizes the same selected native source. It returns
// nil on any error/budget failure. All bytes include provenance and escaping.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := selectMetadata(options)
	if err != nil {
		return nil, err
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql), plan.schema, limits, rowset.EnvelopeOptions{
		MaxRows:    1,
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(&Error{Operation: "decode", cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := decode(result, plan)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "header", "encode":
			return &Error{Operation: "encode", cause: cause}
		case "query", "budget", "consume":
			return &Error{Operation: stage, cause: cause}
		}
		return cause
	})
}

func open(ctx context.Context, q rowset.Queryer, plan selection) (*sql.Rows, error) {
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, &Error{Operation: "query", cause: err}
	}
	return rows, nil
}

func decode(result session.Result, plan selection) (Observation, error) {
	observation := Observation{State: Unavailable, Provenance: plan.provenance}
	fail := func() (Observation, error) {
		return observation, &Error{Operation: "decode", cause: ErrInvalidEvidence}
	}
	names := make([]string, len(plan.schema))
	for i, column := range plan.schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok || len(result.Rows) > 1 {
		return fail()
	}
	if len(result.Rows) == 0 {
		observation.State = NoRows
		return observation, nil
	}
	row := result.Rows[0]
	for _, value := range row {
		if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
			return fail()
		}
	}
	container := row[index["CON_ID"]]
	if !container.Null {
		id, err := strconv.ParseInt(container.Text, 10, 64)
		if err != nil || id != 0 {
			return fail()
		}
	}
	info := &Info{FileName: row[index["FILE_NAME"]], Format: row[index["FORMAT"]], IsASM: row[index["IS_ASM"]], ContainerID: container}
	if position, selected := index["IS_EXC"]; selected {
		value := row[position]
		info.IsExascale = &value
	}
	observation.State, observation.Value = Available, info
	return observation, nil
}

// InfoStatement is the named form of Statement.
func InfoStatement(options Options) (string, error) {
	return Statement(options)
}

// DecodeInfo is the named form of Decode.
func DecodeInfo(result session.Result, options Options) (Observation, error) {
	return Decode(result, options)
}

// ReadInfo is the named form of Read.
func ReadInfo(ctx context.Context, reader Reader, request Request) (Observation, error) {
	return Read(ctx, reader, request)
}

// InfoRows is the named form of Rows.
func InfoRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	return Rows(ctx, q, options)
}

// InfoJSON is the named form of JSON.
func InfoJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options, limits)
}

// Options admits a numeric Oracle release and optional source column. Use
// DefaultOptions for documented release defaults, or explicitly narrow/admit
// the projection. No option probes/retries after a source-query failure.
type Options struct {
	OracleVersion   string
	IncludeExascale bool
}

// Provenance describes a connected-instance metadata observation. CON_ID0 is
// CDB-shared file metadata, not a per-PDB file or complete RAC/file-system proof.
type Provenance struct {
	Source         string `json:"source"`
	OracleVersion  string `json:"oracle_version"`
	Scope          string `json:"scope"`
	ExascaleColumn bool   `json:"exascale_column"`
}

var (
	// ErrUnsupportedVersion rejects releases below12.2 or invalid numeric evidence.
	ErrUnsupportedVersion = errors.New("password-file metadata requires Oracle 12.2.0.1 or later")
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("password-file reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed metadata, values or extra source rows.
	ErrInvalidEvidence = errors.New("invalid Oracle password-file evidence")
)

type selection struct {
	sql        string
	schema     []rowset.Column
	provenance Provenance
}

// Statement selects metadata only. The base projection exists from12.2.0.1;
// optional IS_EXC requires explicit column admission, not a guessed version gate.
func Statement(options Options) (string, error) {
	plan, err := selectMetadata(options)
	return plan.sql, err
}

// DefaultOptions selects documented columns for numeric version evidence:
// the common view from12.2 and IS_EXC from21c. This is a static source contract,
// not a metadata probe or native compatibility certification. Explicit Options
// remain available when the caller admits a narrower or different projection.
func DefaultOptions(oracleVersion string) (Options, error) {
	if !supported(oracleVersion) {
		return Options{}, ErrUnsupportedVersion
	}
	leading, _, _ := strings.Cut(oracleVersion, ".")
	major, _ := strconv.Atoi(leading)
	return Options{OracleVersion: oracleVersion, IncludeExascale: major >= 21}, nil
}

func selectMetadata(options Options) (selection, error) {
	if !supported(options.OracleVersion) {
		return selection{}, ErrUnsupportedVersion
	}
	columns := []rowset.Column{{Name: "FILE_NAME", Kind: rowset.Text}, {Name: "FORMAT", Kind: rowset.Text}, {Name: "IS_ASM", Kind: rowset.Text}}
	if options.IncludeExascale {
		columns = append(columns, rowset.Column{Name: "IS_EXC", Kind: rowset.Text})
	}
	columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "p." + column.Name + " AS " + column.Name
	}
	return selection{
		sql:        "SELECT " + strings.Join(aliases, ", ") + " FROM V$PASSWORDFILE_INFO p",
		schema:     columns,
		provenance: Provenance{Source: "V$PASSWORDFILE_INFO", OracleVersion: options.OracleVersion, Scope: "connected_instance_database_password_file", ExascaleColumn: options.IncludeExascale},
	}, nil
}

func supported(version string) bool {
	if len(version) == 0 || len(version) > 128 {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) > 5 {
		return false
	}
	numbers := [5]int{}
	for i, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		numbers[i] = number
	}
	minimum := [5]int{12, 2, 0, 1, 0}
	for i, number := range numbers {
		if number != minimum[i] {
			return number > minimum[i]
		}
	}
	return true
}
