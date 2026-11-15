package asm

import "github.com/marcuoli/go-sqltemplate/reference"

// SourceRole is caller-observed source context, never inferred from client names.
type SourceRole string

const (
	// DatabaseInstance describes connections from databases with open ASM files.
	DatabaseInstance SourceRole = "database_instance"
	// ASMInstance describes clients using disk groups managed by ASM.
	ASMInstance SourceRole = "asm_instance"
)

// Filter selects reporting IDs and requires explicit source-role admission.
type Filter struct {
	Instances  []int64
	SourceRole SourceRole
}

// Request supplies admitted source version, role, instances and SQL reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}
