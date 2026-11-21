package projection

import (
	"slices"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// ValidateRowset applies the existing domain decoder to a fully consumed
// table, while JSON still encodes the original native values.
func ValidateRowset(invalid error, validate func(session.Result) error) func(rowset.Table) error {
	return func(table rowset.Table) error {
		result, ok := FromRowset(table)
		if !ok {
			return invalid
		}
		return validate(result)
	}
}

// WrapEnvelopeError replaces the shared stage wrapper with the historical
// domain error while retaining the underlying transport or validation cause.
func WrapEnvelopeError(err error, wrap func(stage string, cause error) error) error {
	if failure, ok := err.(*rowset.EnvelopeError); ok {
		return wrap(failure.Stage, failure.Unwrap())
	}
	return err
}

// WrapEnvelopeErrorExcept applies the domain wrapper except at stages with
// historically unwrapped errors.
func WrapEnvelopeErrorExcept(err error, wrap func(error) error, rawStages ...string) error {
	return WrapEnvelopeError(err, func(stage string, cause error) error {
		if slices.Contains(rawStages, stage) {
			return cause
		}
		return wrap(cause)
	})
}

// WrapColumnEnvelopeError uses the published operation names of the
// column-validated dictionary inventories.
func WrapColumnEnvelopeError(err error, wrap func(string, error) error) error {
	return WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "limits":
			return cause
		case "header":
			stage = "encode"
		case "validate":
			stage = "decode"
		}
		return wrap(stage, cause)
	})
}
