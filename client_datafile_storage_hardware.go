package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/asm"
)

// DatafileStorageHardware transfers the native cursor for the connected
// database's ASM datafile-group probe. Use a non-CDB or CDB root connection.
func (c *Client) DatafileStorageHardware(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return asm.DatafileStorageHardwareRows(ctx, c.native(), version)
}

// DatafileStorageHardwareJSON reads the hardware probe through a separate
// bounded cursor. A zero count leaves the hardware type unknown.
func (c *Client) DatafileStorageHardwareJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return asm.DatafileStorageHardwareJSON(ctx, c.native(), version, resolved)
}
