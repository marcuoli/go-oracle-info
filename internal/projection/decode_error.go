package projection

import "errors"

// decodeError lets a compatibility adapter translate errors from its own
// decoder without interpreting failures returned by the caller's reader.
type decodeError struct{ error }

func (e decodeError) Unwrap() error { return e.error }

// MarkDecodeError marks a failure from a moved fact's local decoder.
func MarkDecodeError(err error) error {
	if err == nil {
		return nil
	}
	return decodeError{err}
}

// MapDecodeError restores legacy errors at the decoder boundary. Buffered
// readers may join the marked failure with cancellation before wrapping it.
func MapDecodeError(err error, mapError func(error) error) error {
	switch cause := err.(type) {
	case decodeError:
		return mapError(cause.error)
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		for i, child := range children {
			if decoded, ok := child.(decodeError); ok {
				mapped := append([]error(nil), children...)
				mapped[i] = mapError(decoded.error)
				return errors.Join(mapped...)
			}
		}
	}
	return err
}
