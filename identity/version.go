package identity

import (
	"context"

	"github.com/marcuoli/go-sqltemplate/reference"
)

// VersionReferences supplies distinct caller-owned bootstrap and full-version SQL sites.
type VersionReferences struct {
	Legacy reference.Reference
	Full   reference.Reference
}

// VersionPrecision identifies the actual selected VERSION or VERSION_FULL source,
// independent of how many nonzero numeric segments its value contains.
type VersionPrecision string

const (
	LegacyPrecision VersionPrecision = "legacy"
	FullPrecision   VersionPrecision = "full"
)

// VersionObservation retains the selected numeric source string unchanged.
// Full is nil without a decoded full observation; failed reads remain in Attempts.
// Legacy precision never claims an observed RU or VERSION_FULL value.
type VersionObservation struct {
	PropertyEvidence
	Value     string
	Precision VersionPrecision
	Legacy    Text
	Full      *Text
}

// ReadVersion bootstraps VERSION and reads VERSION_FULL once on established18+.
// Missing/failed full evidence can retain the independently valid legacy value,
// with Fallback and legacy Precision. Errors remain returned, including cancellation;
// that reuse is not another query or a successful full-version claim. Contradictory
// numeric majors make the property unavailable while preserving both raw values.
func ReadVersion(ctx context.Context, reader Reader, refs VersionReferences) (VersionObservation, error) {
	observation := VersionObservation{
		PropertyEvidence: PropertyEvidence{State: Unavailable, Source: InstanceVersionSource},
		Legacy:           Text{State: Unavailable},
	}
	if err := validatePropertyReader(reader, refs.Legacy, refs.Full); err != nil {
		return observation, err
	}
	legacy, major, err := bootstrapProperty(ctx, reader, refs.Legacy, &observation.Attempts)
	observation.Legacy = legacy
	if err != nil {
		return observation, err
	}
	observation.State = legacy.State
	if legacy.State != Available {
		return observation, nil
	}
	observation.Value, observation.Precision = legacy.Value, LegacyPrecision
	if major < 18 {
		return observation, nil
	}
	statement, err := VersionStatement(major)
	if err != nil {
		return observation, err
	}
	full, err := scalarProperty(ctx, reader, refs.Full, statement, InstanceFullVersionSource, DecodeVersion, &observation.Attempts)
	if err != nil {
		observation.Fallback = true
		return observation, err
	}
	observation.Full = &full
	if full.State != Available {
		observation.Fallback = true
		return observation, nil
	}
	fullMajor, err := numericVersionMajor(full.Value)
	if err != nil || fullMajor != major {
		observation.State, observation.Value, observation.Precision = Unavailable, "", ""
		if err == nil {
			err = ErrInvalidEvidence
		}
		return observation, propertyDecodeFailure(InstanceFullVersionSource, err, &observation.Attempts)
	}
	observation.Value, observation.Precision = full.Value, FullPrecision
	observation.Source = InstanceFullVersionSource
	return observation, nil
}
