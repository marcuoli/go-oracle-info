package identity

// DatabaseInfoOptions admits the established source release. IncludeSupplementalPL
// must be true only when the caller established this column on that exact source;
// nil SupplementalPL means unselected, not a selected SQL NULL or NO.
type DatabaseInfoOptions struct {
	OracleMajor           int
	IncludeSupplementalPL bool
}
