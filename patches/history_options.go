// Package patches selects documented patch and upgrade metadata without patching
// the database. History and datapatch installation attempts remain separate.
package patches

// HistoryOptions admits BUNDLE_SERIES independently for older patch-set sources.
// A nil BundleSeries is unselected; a nonnil Null value is selected SQL NULL.
type HistoryOptions struct {
	OracleMajor         int
	IncludeBundleSeries bool
}

// SQLPatchOptions distinguishes the documented 12.1, 12.2 and 18+ projections.
// OracleMinor must be 1 or 2 for 12; 18+ requires zero because its major selects
// the modern metadata layout. XML descriptors and binary patch payloads are not read.
type SQLPatchOptions struct {
	OracleMajor int
	OracleMinor int
}
