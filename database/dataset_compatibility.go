package database

import (
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-info/storage"
)

// Each adapter restores the database package's published errors while retaining
// the original native query, driver and cancellation causes.
func legacyMemoryDatasetError(err error) error {
	err, decoded := projection.UnwrapDecodeError(err)
	switch err {
	case memory.ErrUnsupportedVersion:
		return ErrUnsupportedVersion
	case memory.ErrInvalidRequest:
		return ErrInvalidRequest
	case memory.ErrInvalidObservedEvidence:
		return ErrInvalidEvidence
	}
	if failure, ok := err.(*memory.DatasetError); ok {
		cause := failure.Unwrap()
		if decoded {
			cause = projection.MapDecodeCause(cause, legacyMemoryDatasetError)
		}
		return &DatasetError{failure.Source, cause}
	}
	return err
}

func legacyStorageDatasetError(err error) error {
	err, decoded := projection.UnwrapDecodeError(err)
	switch err {
	case storage.ErrUnsupportedVersion:
		return ErrUnsupportedVersion
	case storage.ErrInvalidRequest:
		return ErrInvalidRequest
	case storage.ErrInvalidEvidence:
		return ErrInvalidEvidence
	}
	if failure, ok := err.(*storage.Error); ok {
		cause := failure.Unwrap()
		if decoded {
			cause = projection.MapDecodeCause(cause, legacyStorageDatasetError)
		}
		return &DatasetError{failure.Source, cause}
	}
	return err
}
