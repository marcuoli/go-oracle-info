package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/parameters"
)

// HiddenCatalogFilter selects fixed-table underscore parameters by exact names
// and/or a literal name substring. Empty selection retains all exposed rows.
type HiddenCatalogFilter = parameters.HiddenCatalogFilter

// HiddenParameterCatalog returns the caller's native session/instance catalog
// cursor after observing the source version. The caller owns Next/Scan/Err/Close
// and budgets. This opt-in fixed-table API never falls back to visible parameters.
func (c *Client) HiddenParameterCatalog(ctx context.Context, filter HiddenCatalogFilter) (*sql.Rows, error) {
	if _, _, err := parameters.HiddenCatalogStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenCatalogRows(ctx, c.native(), major, filter)
}

// HiddenParameterCatalogJSON independently consumes the same fixed-table
// selection into bounded JSON, preserving textual values and SQL NULL.
func (c *Client) HiddenParameterCatalogJSON(ctx context.Context, filter HiddenCatalogFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := parameters.HiddenCatalogStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenCatalogJSON(ctx, c.native(), major, filter, resolved)
}

// HiddenParameterCatalogWithSessionDefault returns the native fixed-table cursor
// with the raw SESSION_IS_DEFAULT column. The caller owns the cursor and budgets.
func (c *Client) HiddenParameterCatalogWithSessionDefault(ctx context.Context, filter HiddenCatalogFilter) (*sql.Rows, error) {
	if _, _, err := parameters.HiddenCatalogWithSessionDefaultStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenCatalogWithSessionDefaultRows(ctx, c.native(), major, filter)
}

// HiddenParameterCatalogWithSessionDefaultJSON consumes the extended catalog
// independently into bounded JSON, preserving the raw flag and SQL NULL.
func (c *Client) HiddenParameterCatalogWithSessionDefaultJSON(ctx context.Context, filter HiddenCatalogFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := parameters.HiddenCatalogWithSessionDefaultStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenCatalogWithSessionDefaultJSON(ctx, c.native(), major, filter, resolved)
}
