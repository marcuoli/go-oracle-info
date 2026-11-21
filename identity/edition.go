package identity

import (
	"context"

	"github.com/marcuoli/go-sqltemplate/reference"
)

// EditionReferences supplies distinct bootstrap, observed-code and legacy-banner sites.
type EditionReferences struct {
	Version reference.Reference
	Code    reference.Reference
	Banners reference.Reference
}

// EditionType classifies explicit Oracle evidence without granting licensing or features.
type EditionType string

const (
	EnterpriseEdition         EditionType = "EE"
	StandardEdition           EditionType = "SE"
	StandardEditionFamily     EditionType = "SE_OR_SE2"
	ExpressEdition            EditionType = "XE"
	FreeEdition               EditionType = "FREE"
	PersonalEdition           EditionType = "PO"
	HighPerformanceEdition    EditionType = "HP"
	ExtremePerformanceEdition EditionType = "XP"
)

// Historical12.1 CORE codes remain distinct observations.
const (
	CoreEnterpriseEdition EditionType = "CORE EE"
	CoreStandardEdition   EditionType = "CORE SE"
)

// EditionPrecision distinguishes an observed EDITION code from an explicit legacy label.
// Raw Code/Label remain separate; a classification never fabricates the other source.
type EditionPrecision string

const (
	ObservedEditionCode EditionPrecision = "observed_code"
	ExplicitLegacyLabel EditionPrecision = "explicit_legacy_label"
)

// EditionObservation preserves raw code or explicit label and legacy banners.
// SE2 code maps to SE_OR_SE2 because Oracle documents that code for either
// Standard Edition or Standard Edition2. No value establishes licensing.
type EditionObservation struct {
	PropertyEvidence
	Value     EditionType
	Precision EditionPrecision
	Version   Text
	Code      Text
	Label     Text
	Banners   []VersionBanner
}

// ReadEdition reads EDITION on12+ or one conservative explicit11g database banner
// description. Unknown evidence remains unavailable and raw; a failed code read
// never triggers a banner query or another attempt on a possibly retired reader.
func ReadEdition(ctx context.Context, reader Reader, refs EditionReferences) (EditionObservation, error) {
	unavailable := Text{State: Unavailable}
	observation := EditionObservation{
		PropertyEvidence: PropertyEvidence{State: Unavailable, Source: InstanceVersionSource},
		Version:          unavailable, Code: unavailable, Label: unavailable,
	}
	if err := validatePropertyReader(reader, refs.Version, refs.Code, refs.Banners); err != nil {
		return observation, err
	}
	version, major, err := bootstrapProperty(ctx, reader, refs.Version, &observation.Attempts)
	observation.Version = version
	if err != nil || version.State != Available {
		return observation, err
	}
	if major >= 12 {
		observation.Source, observation.Precision = InstanceEditionSource, ObservedEditionCode
		statement, err := EditionStatement(major)
		if err != nil {
			return observation, err
		}
		code, err := scalarProperty(ctx, reader, refs.Code, statement, InstanceEditionSource, DecodeEdition, &observation.Attempts)
		observation.Code = code
		if err != nil {
			return observation, err
		}
		observation.State = code.State
		if code.State != Available {
			return observation, nil
		}
		switch EditionType(code.Value) {
		case EnterpriseEdition, StandardEdition, ExpressEdition, FreeEdition, PersonalEdition, HighPerformanceEdition, ExtremePerformanceEdition, CoreEnterpriseEdition, CoreStandardEdition:
			observation.Value = EditionType(code.Value)
		case "SE2":
			observation.Value = StandardEditionFamily
		default:
			observation.State = Unavailable
			return observation, propertyDecodeFailure(InstanceEditionSource, ErrUnknownEdition, &observation.Attempts)
		}
		return observation, nil
	}
	observation.Source, observation.Precision, observation.Fallback = VersionBannersSource, ExplicitLegacyLabel, true
	statement, err := BannersStatement(major)
	if err != nil {
		return observation, err
	}
	result, err := queryProperty(ctx, reader, refs.Banners, statement, VersionBannersSource, &observation.Attempts)
	if err != nil {
		return observation, err
	}
	// This is a complete-source bound, not permission to accept a truncated prefix.
	if len(result.Rows) > 16 {
		return observation, propertyDecodeFailure(VersionBannersSource, ErrInvalidEvidence, &observation.Attempts)
	}
	banners, err := DecodeBanners(result, major)
	if err != nil {
		return observation, propertyDecodeFailure(VersionBannersSource, err, &observation.Attempts)
	}
	observation.Banners = banners
	if len(banners) == 0 {
		observation.State = NoRows
		observation.Attempts[len(observation.Attempts)-1].State = NoRows
		return observation, nil
	}
	observation.Attempts[len(observation.Attempts)-1].State = Available
	description, ok := LegacyEditionDescription(banners)
	if !ok {
		return observation, propertyDecodeFailure(VersionBannersSource, ErrUnknownEdition, &observation.Attempts)
	}
	observation.Label = Text{State: Available, Value: description.Description}
	switch description.Description {
	case "Enterprise Edition":
		observation.Value = EnterpriseEdition
	case "Standard Edition":
		observation.Value = StandardEdition
	case "Express Edition":
		observation.Value = ExpressEdition
	}
	observation.State = Available
	return observation, nil
}
