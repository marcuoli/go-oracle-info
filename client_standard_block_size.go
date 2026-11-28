package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/database"
)

// StandardBlockSize transfers the native V$PARAMETER cursor for the admitted database.
func (c *Client) StandardBlockSize(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.StandardBlockSizeRows(ctx, c.native(), database.StandardBlockSizeOptions{OracleVersion: version})
}

// StandardBlockSizeJSON reads a separate complete bounded V$PARAMETER cursor.
func (c *Client) StandardBlockSizeJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.StandardBlockSizeJSON(ctx, c.native(), database.StandardBlockSizeOptions{OracleVersion: version}, resolved)
}
