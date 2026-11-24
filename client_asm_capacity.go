package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/asm"
)

// ASMDiskGroupCapacity transfers the native cursor for the caller-admitted
// database or ASM instance. The role comes from the caller's scope evidence.
func (c *Client) ASMDiskGroupCapacity(ctx context.Context, sourceRole asm.SourceRole) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return asm.DiskGroupCapacityRows(ctx, c.native(), asm.DiskGroupCapacityOptions{OracleVersion: version, SourceRole: sourceRole})
}

// ASMDiskGroupCapacityJSON consumes a separate complete bounded cursor.
func (c *Client) ASMDiskGroupCapacityJSON(ctx context.Context, sourceRole asm.SourceRole, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return asm.DiskGroupCapacityJSON(ctx, c.native(), asm.DiskGroupCapacityOptions{OracleVersion: version, SourceRole: sourceRole}, resolved)
}
