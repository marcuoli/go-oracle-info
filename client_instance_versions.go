package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/instances"
)

// InstanceVersions returns every qualified visible member's native version row.
// It owns no member connections and never falls back to a local-only inventory.
func (c *Client) InstanceVersions(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return instances.VersionsRows(ctx, c.native(), major)
}

// InstanceVersionsJSON independently consumes complete member versions with the
// same source and finite limits. Legacy VERSION is never labeled VERSION_FULL.
func (c *Client) InstanceVersionsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return instances.VersionsJSON(ctx, c.native(), major, resolved)
}
