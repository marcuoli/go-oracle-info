package storage

import "github.com/marcuoli/go-sqltemplate/reference"

// Request admits a source version and caller-owned SQL execution reference.
type Request struct {
	OracleMajor int
	Reference   reference.Reference
}

// TempfileOptions explicitly admits optional local-tempfile columns. IncludeLocal
// requires 18+; the common 11g projection never fabricates owning-instance data.
type TempfileOptions struct {
	OracleMajor  int
	IncludeLocal bool
}

// TempfileRequest supplies the explicit column capability and caller reference.
type TempfileRequest struct {
	Options   TempfileOptions
	Reference reference.Reference
}

// InstanceFilter selects reporting IDs; empty means all visible admitted rows.
type InstanceFilter struct{ Instances []int64 }

// SGARequest admits per-instance observed component facts, not configured targets.
type SGARequest struct {
	OracleMajor int
	Filter      InstanceFilter
	Reference   reference.Reference
}
