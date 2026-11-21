package projection

import (
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// ErrSourceInaccessible marks a query that Oracle rejected for insufficient privilege.
var ErrSourceInaccessible = errors.New("Oracle fact source is inaccessible")

// ErrSourceUnavailable marks a query that Oracle could not resolve or admit
// in the observed database state. An unresolvable view does not prove missing grants.
var ErrSourceUnavailable = errors.New("Oracle fact source is unavailable")

// ClassifySourceError recognizes a single Oracle source diagnostic. A multi-cause
// failure may also carry cancellation, cursor-close or transport evidence.
func ClassifySourceError(err error) error {
	current := err
	var reported error
	for depth := 0; current != nil && depth < 64; depth++ {
		if owned, ok := current.(*session.Error); ok && owned != nil && reported == nil {
			reported = sourceErrorClass(owned.Code)
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			causes := wrapped.Unwrap()
			if len(causes) != 1 {
				return err
			}
			current = causes[0]
			continue
		case interface{ Unwrap() error }:
			if cause := wrapped.Unwrap(); cause != nil {
				current = cause
				continue
			}
		}
		if reported == nil {
			reported = sourceErrorClass(current.Error())
		}
		if reported != nil {
			return errors.Join(reported, err)
		}
		return err
	}
	return err
}

func sourceErrorClass(message string) error {
	code := strings.TrimSpace(message)
	if end := strings.IndexAny(code, ": \t\r\n"); end >= 0 {
		code = code[:end]
	}
	switch code {
	case "ORA-01031", "ORA-1031":
		return ErrSourceInaccessible
	case "ORA-00942", "ORA-942", "ORA-01109", "ORA-1109", "ORA-01219", "ORA-1219", "ORA-01507", "ORA-1507":
		return ErrSourceUnavailable
	}
	return nil
}
