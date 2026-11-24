package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/objects"
)

// TimeZoneColumns transfers the caller-native inventory cursor.
func (c *Client) TimeZoneColumns(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.TimeZoneColumnsRows(ctx, c.native(), objects.TimeZoneColumnsOptions{OracleVersion: version})
}

// TimeZoneColumnsJSON consumes a separate complete bounded inventory cursor.
func (c *Client) TimeZoneColumnsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.TimeZoneColumnsJSON(ctx, c.native(), objects.TimeZoneColumnsOptions{OracleVersion: version}, resolved)
}
