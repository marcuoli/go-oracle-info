package identity

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Unavailable means no justified property value could be established. Raw facts,
// selected SQL NULL and successful no-row observations remain separately retained.
const Unavailable State = "unavailable"

// Reader is an already-admitted, caller-owned reader of complete results. It must
// retain terminal/close failures and enforce budgets, never return a failed prefix.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// PropertySource identifies a reviewed dictionary column or atomic projection.
type PropertySource string

const (
	InstanceVersionSource     PropertySource = "v$instance.version"
	InstanceFullVersionSource PropertySource = "v$instance.version_full"
	InstanceEditionSource     PropertySource = "v$instance.edition"
	ConnectedContainerSource  PropertySource = "v$database.cdb+sys_context.current_container"
	VersionBannersSource      PropertySource = "v$version.banner"
)

// PropertyAttempt records an actual reader call, including decoding failure.
// Err has a safe static message; explicit errors.Is/As/Unwrap inspection retains
// the original cause for private diagnostics. State is not transport completeness.
type PropertyAttempt struct {
	Source PropertySource
	State  State
	Err    error
}

// PropertyEvidence describes the selected value, not a destination admission.
// Fallback identifies a legacy compatibility source or reuse of a valid bootstrap
// value. Attempts preserve execution order and every failure; no attempt is retried.
type PropertyEvidence struct {
	State    State
	Source   PropertySource
	Fallback bool
	Attempts []PropertyAttempt
}

var (
	ErrInvalidPropertyRequest = errors.New("identity reader and distinct valid references are required")
	ErrUnknownEdition         = errors.New("unrecognized explicit Oracle edition evidence")
)

// PropertyError prevents driver text from appearing in its message. Unwrap is
// intentional: callers may inspect the original cause in private diagnostics.
type PropertyError struct {
	Source    PropertySource
	Operation string
	cause     error
}

// Error returns source/operation context without formatting the original cause.
func (e *PropertyError) Error() string {
	return "identity property " + e.Operation + " failed (" + string(e.Source) + ")"
}

// Unwrap exposes the original cause for explicit private inspection and errors.Is/As.
func (e *PropertyError) Unwrap() error { return e.cause }

func validatePropertyReader(reader Reader, refs ...reference.Reference) error {
	if reader == nil {
		return ErrInvalidPropertyRequest
	}
	seen := make(map[reference.Reference]bool, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil || seen[ref] {
			return ErrInvalidPropertyRequest
		}
		seen[ref] = true
	}
	return nil
}

func queryProperty(ctx context.Context, reader Reader, ref reference.Reference, statement string, source PropertySource, attempts *[]PropertyAttempt) (session.Result, error) {
	if err := ctx.Err(); err != nil {
		return session.Result{}, &PropertyError{Source: source, Operation: "query", cause: err}
	}
	*attempts = append(*attempts, PropertyAttempt{Source: source, State: Unavailable})
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: statement})
	// A reader returning a payload after cancellation has not established a
	// usable observation. Retain both causes, never continue on that payload.
	if canceled := ctx.Err(); canceled != nil {
		err = errors.Join(err, canceled)
	}
	if err != nil {
		failure := &PropertyError{Source: source, Operation: "query", cause: err}
		(*attempts)[len(*attempts)-1].Err = failure
		return session.Result{}, failure
	}
	return result, nil
}

func propertyDecodeFailure(source PropertySource, cause error, attempts *[]PropertyAttempt) error {
	failure := &PropertyError{Source: source, Operation: "decode", cause: cause}
	last := &(*attempts)[len(*attempts)-1]
	last.State, last.Err = Unavailable, failure
	return failure
}

func scalarProperty(ctx context.Context, reader Reader, ref reference.Reference, statement string, source PropertySource, decode func(session.Result) (Text, error), attempts *[]PropertyAttempt) (Text, error) {
	result, err := queryProperty(ctx, reader, ref, statement, source, attempts)
	if err != nil {
		return Text{State: Unavailable}, err
	}
	value, err := decode(result)
	if err != nil {
		return Text{State: Unavailable}, propertyDecodeFailure(source, err, attempts)
	}
	(*attempts)[len(*attempts)-1].State = value.State
	return value, nil
}

func bootstrapProperty(ctx context.Context, reader Reader, ref reference.Reference, attempts *[]PropertyAttempt) (Text, int, error) {
	statement, err := VersionStatement(0)
	if err != nil {
		return Text{State: Unavailable}, 0, err
	}
	value, err := scalarProperty(ctx, reader, ref, statement, InstanceVersionSource, DecodeVersion, attempts)
	if err != nil || value.State != Available {
		return value, 0, err
	}
	major, err := numericVersionMajor(value.Value)
	if err != nil {
		return value, 0, propertyDecodeFailure(InstanceVersionSource, err, attempts)
	}
	return value, major, nil
}

// Numeric source versions have five decimal segments. Marketing names, padding,
// signs, extra segments and oversized values cannot select a source capability.
func numericVersionMajor(value string) (int, error) {
	if len(value) > 128 {
		return 0, ErrInvalidEvidence
	}
	parts := strings.Split(value, ".")
	if len(parts) != 5 {
		return 0, ErrInvalidEvidence
	}
	var major int
	for i, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, ErrInvalidEvidence
		}
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return 0, ErrInvalidEvidence
		}
		if i == 0 {
			major = int(number)
		}
	}
	if major < 11 {
		return 0, ErrUnsupportedVersion
	}
	return major, nil
}
