package database

import (
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-info/storage"
)

// Each adapter restores the database package's published errors while retaining
// the original native query, driver and cancellation causes.
func legacyMemoryDatasetError(err error) error {
	switch err {
	case memory.ErrUnsupportedVersion:
		return ErrUnsupportedVersion
	case memory.ErrInvalidRequest:
		return ErrInvalidRequest
	case memory.ErrInvalidObservedEvidence:
		return ErrInvalidEvidence
	}
	if failure, ok := err.(*memory.DatasetError); ok {
		return &DatasetError{failure.Source, projection.MapDecodeError(failure.Unwrap(), legacyMemoryDatasetError)}
	}
	return err
}

func legacyStorageDatasetError(err error) error {
	switch err {
	case storage.ErrUnsupportedVersion:
		return ErrUnsupportedVersion
	case storage.ErrInvalidRequest:
		return ErrInvalidRequest
	case storage.ErrInvalidEvidence:
		return ErrInvalidEvidence
	}
	if failure, ok := err.(*storage.Error); ok {
		return &DatasetError{failure.Source, projection.MapDecodeError(failure.Unwrap(), legacyStorageDatasetError)}
	}
	return err
}
