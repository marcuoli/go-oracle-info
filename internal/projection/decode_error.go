package projection

import (
	"errors"
	"slices"
)

// decodeError identifies the owning operation's validation failure. It marks
// the top-level result so adapters never infer origin from a reader's causes.
type decodeError struct{ error }

func (e decodeError) Unwrap() error { return e.error }

// MarkDecodeError marks a wrapped operation error when its decoder failed.
func MarkDecodeError(err, decodeErr error) error {
	if decodeErr == nil {
		return err
	}
	return decodeError{err}
}

// UnwrapDecodeError checks only the current operation's outer error.
func UnwrapDecodeError(err error) (error, bool) {
	if failure, ok := err.(decodeError); ok {
		return failure.error, true
	}
	return err, false
}

// MapDecodeCause adapts a known decoder failure. When joined with cancellation
// by a buffered reader, the decoder failure is the first non-nil cause.
func MapDecodeCause(cause error, mapError func(error) error) error {
	if joined, ok := cause.(interface{ Unwrap() []error }); ok {
		children := append([]error(nil), joined.Unwrap()...)
		children[0] = mapError(children[0])
		return errors.Join(children...)
	}
	return mapError(cause)
}

// WrapMovedEnvelopeError retains the shared envelope's validation origin at
// the domain boundary for the moved fact's compatibility adapter.
func WrapMovedEnvelopeError(err error, wrap func(error) error, rawStages ...string) error {
	return WrapEnvelopeError(err, func(stage string, cause error) error {
		if slices.Contains(rawStages, stage) {
			return cause
		}
		failure := wrap(cause)
		if stage == "validate" {
			return MarkDecodeError(failure, cause)
		}
		return failure
	})
}
