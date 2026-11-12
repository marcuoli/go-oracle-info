// Package engineeredsystem owns observed database-visible cell evidence, not
// connections or hardware-family/health inference. XML source bytes are retained
// independently of decoded properties so a failed document never erases evidence.
package engineeredsystem

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Source string

const (
	Cells      Source = "cells"
	Config     Source = "config"
	ConfigInfo Source = "config_info"
	OpenAlerts Source = "open_alerts"
)

func Sources() []Source { return []Source{Cells, Config, ConfigInfo, OpenAlerts} }

type State string

const (
	Available    State = "available"
	Empty        State = "empty"
	Unavailable  State = "unavailable"
	Unsupported  State = "unsupported"
	Shared       State = "shared"
	NotCollected State = "not_collected"
	Failed       State = "failed"
)

func States() []State {
	return []State{Available, Empty, Unavailable, Unsupported, Shared, NotCollected, Failed}
}

type Reason string

const (
	ReasonNone                 Reason = ""
	ReasonNotVisible           Reason = "source_not_visible"
	ReasonUnknownSchema        Reason = "unknown_schema"
	ReasonNoCells              Reason = "no_cells"
	ReasonNoRows               Reason = "no_rows"
	ReasonVersionUnsupported   Reason = "version_unsupported"
	ReasonSharedInfrastructure Reason = "shared_infrastructure"
	ReasonReadFailed           Reason = "read_failed"
	ReasonInvalidPayload       Reason = "invalid_payload"
	ReasonNotCollected         Reason = "not_collected"
	ReasonContainerUnavailable Reason = "container_unavailable"
	ReasonConflictingEvidence  Reason = "conflicting_evidence"
	ReasonAlternateSource      Reason = "alternate_source_selected"
	ReasonCurrentInstanceOnly  Reason = "current_instance_only"
)

func Reasons() []Reason {
	return []Reason{ReasonNone, ReasonNotVisible, ReasonUnknownSchema, ReasonNoCells, ReasonNoRows, ReasonVersionUnsupported, ReasonSharedInfrastructure, ReasonReadFailed, ReasonInvalidPayload, ReasonNotCollected, ReasonContainerUnavailable, ReasonConflictingEvidence, ReasonAlternateSource, ReasonCurrentInstanceOnly}
}

// Coverage counts source rows, not their potentially many XML properties. Valid
// and rejected rows are disjoint. A rejected row's original Payload may still be
// retained, including malformed XML. View identifies the actual GV$/V$ variant.
// Detail describes an expected limitation; Error records an actual failure.
type Coverage struct {
	Source                               Source
	State                                State
	Reason                               Reason
	View, Detail, Error                  string
	ObservedAt                           time.Time
	RowsRead, RowsRetained, RowsRejected int64
	OmittedUnknown                       bool
	// InstanceLocal is independent of failure/availability: a V$ read can retain
	// local rows and still fail, so its scope must survive the failure reason.
	InstanceLocal bool
}

// Cell descriptors retain their source SQL NULL independently of identity.
// A missing type/hash/path must not become an invented empty-string observation.
type Cell struct {
	InstanceID                   int64
	Hash, Path, Type             string
	HashNull, PathNull, TypeNull bool
}

// Property preserves source values exactly; Unit and ReportedStatus are reported
// metadata, never calculated rates, inferred health or parsed floating-point data.
// Name is a namespace-preserving XML path for XML values. Ordinal retains source
// order without deduplicating repeated or conflicting observations.
type Property struct {
	InstanceID                        int64
	CellHash, CellPath, CellType      string
	ComponentType, ComponentID        string
	Name, Value, Unit, ReportedStatus string
	Source                            Source
	SourceType                        string
	Null                              bool
	ObservedAt                        time.Time
	Ordinal                           int
}

// XML deliberately permits invalid UTF-8: a decoder failure must not lose raw
// source bytes. Artifact encoders must write those bytes directly or base64 them;
// encoding an invalid XML string directly through JSON would replace its bytes.
// Null distinguishes SQL NULL from an empty, invalid XML document.
type Payload struct {
	InstanceID                   int64
	CellHash, CellPath, CellType string
	SourceType, ObjectName       string
	Rejected                     bool
	Reason                       Reason
	Error                        string
	NativeLength                 int64
	NativeLengthKnown            bool
	XML                          string `json:"-"`
	Source                       Source
	ObservedAt                   time.Time
	Null                         bool
}

// MarshalJSON retains XML bytes through assessment snapshots as well as output
// artifacts. JSON string encoding would silently replace invalid UTF-8 from a
// rejected payload; base64 is an explicit byte-preserving representation.
func (p Payload) MarshalJSON() ([]byte, error) {
	type fields Payload
	return json.Marshal(struct {
		fields
		XMLBytes []byte `json:"xml_bytes"`
	}{fields: fields(p), XMLBytes: []byte(p.XML)})
}

func (p *Payload) UnmarshalJSON(data []byte) error {
	type fields Payload
	var wire struct {
		fields
		XMLBytes *[]byte `json:"xml_bytes"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.XMLBytes == nil {
		return fmt.Errorf("engineered-system payload lacks encoded XML bytes")
	}
	*p = Payload(wire.fields)
	p.XML = string(*wire.XMLBytes)
	return nil
}

type Report struct {
	Cells      []Cell
	Properties []Property
	Payloads   []Payload
	Sources    []Coverage
}

func (r Report) Clone() Report {
	r.Cells = slices.Clone(r.Cells)
	r.Properties = slices.Clone(r.Properties)
	r.Payloads = slices.Clone(r.Payloads)
	r.Sources = slices.Clone(r.Sources)
	return r
}

func validText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

// Validate checks evidence consistency without classifying a hardware family,
// selecting a winning duplicate, changing NULLs or dropping unsupported fields.
func (r Report) Validate() error {
	coverage := make(map[Source]Coverage, len(r.Sources))
	for _, source := range r.Sources {
		if !slices.Contains(Sources(), source.Source) || !slices.Contains(States(), source.State) || !slices.Contains(Reasons(), source.Reason) {
			return fmt.Errorf("invalid engineered-system source/state/reason")
		}
		if _, exists := coverage[source.Source]; exists {
			return fmt.Errorf("duplicate engineered-system source coverage %q", source.Source)
		}
		if source.RowsRead < 0 || source.RowsRetained < 0 || source.RowsRejected < 0 || source.RowsRejected > source.RowsRead || source.RowsRetained > source.RowsRead-source.RowsRejected {
			return fmt.Errorf("invalid engineered-system source row counts for %q", source.Source)
		}
		if !validText(source.View, source.Detail, source.Error) {
			return fmt.Errorf("invalid engineered-system source text")
		}
		if source.State == Failed && source.Error == "" {
			return fmt.Errorf("failed engineered-system source %q lacks an error", source.Source)
		}
		if source.State != Failed && source.Error != "" {
			return fmt.Errorf("engineered-system source %q hides a failure in an expected state", source.Source)
		}
		if source.State != Available && source.State != Failed && source.RowsRetained != 0 {
			return fmt.Errorf("unavailable engineered-system source %q retains accepted rows", source.Source)
		}
		if source.State == Available && source.Reason != ReasonNone && source.Reason != ReasonCurrentInstanceOnly {
			return fmt.Errorf("available engineered-system source %q has an unavailability reason", source.Source)
		}
		coverage[source.Source] = source
	}
	requireObserved := func(source Source) error {
		c, exists := coverage[source]
		if !exists || (c.State != Available && c.State != Failed) {
			return fmt.Errorf("engineered-system observation has no readable source coverage %q", source)
		}
		return nil
	}
	for _, cell := range r.Cells {
		if cell.InstanceID < 0 || (strings.TrimSpace(cell.Hash) == "" && strings.TrimSpace(cell.Path) == "") || !validText(cell.Hash, cell.Path, cell.Type) || (cell.HashNull && cell.Hash != "") || (cell.PathNull && cell.Path != "") || (cell.TypeNull && cell.Type != "") {
			return fmt.Errorf("invalid engineered-system cell identity")
		}
		if err := requireObserved(Cells); err != nil {
			return err
		}
	}
	for _, p := range r.Properties {
		if p.InstanceID < 0 || p.Ordinal < 0 || strings.TrimSpace(p.Name) == "" || !slices.Contains(Sources(), p.Source) || !validText(p.CellHash, p.CellPath, p.CellType, p.ComponentType, p.ComponentID, p.Name, p.Value, p.Unit, p.ReportedStatus, p.SourceType) || (p.Null && p.Value != "") {
			return fmt.Errorf("invalid engineered-system property")
		}
		if err := requireObserved(p.Source); err != nil {
			return err
		}
	}
	for _, payload := range r.Payloads {
		if payload.InstanceID < 0 || !slices.Contains(Sources(), payload.Source) || !slices.Contains(Reasons(), payload.Reason) || !validText(payload.CellHash, payload.CellPath, payload.CellType, payload.SourceType, payload.ObjectName, payload.Error) || (payload.Null && payload.XML != "") || (!payload.NativeLengthKnown && payload.NativeLength != 0) {
			return fmt.Errorf("invalid engineered-system payload metadata")
		}
		if payload.Rejected && (payload.Error == "" || payload.Reason == ReasonNone) {
			return fmt.Errorf("rejected engineered-system payload lacks its rejection evidence")
		}
		if !payload.Rejected && (payload.Error != "" || payload.Reason != ReasonNone) {
			return fmt.Errorf("accepted engineered-system payload carries an undisclosed rejection")
		}
		if err := requireObserved(payload.Source); err != nil {
			return err
		}
	}
	return nil
}
