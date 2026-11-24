package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/redo"
	"github.com/marcuoli/go-oracle-info/services"
)

// ObjectIdentities returns the actual native cursor for the admitted observation.
func (c *Client) ObjectIdentities(ctx context.Context, owners []string) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.ObjectIdentitiesRows(ctx, c.native(), objects.ObjectIdentitiesOptions{OracleVersion: version, Owners: owners})
}

// ObjectIdentitiesJSON reads a separate complete bounded cursor.
func (c *Client) ObjectIdentitiesJSON(ctx context.Context, owners []string, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.ObjectIdentitiesJSON(ctx, c.native(), objects.ObjectIdentitiesOptions{OracleVersion: version, Owners: owners}, resolved)
}

// RedoThreads returns the actual native cursor for the admitted observation.
func (c *Client) RedoThreads(ctx context.Context, databaseRole string) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ThreadsRows(ctx, c.native(), redo.ThreadsOptions{OracleVersion: version, DatabaseRole: databaseRole})
}

// RedoThreadsJSON reads a separate complete bounded cursor.
func (c *Client) RedoThreadsJSON(ctx context.Context, databaseRole string, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ThreadsJSON(ctx, c.native(), redo.ThreadsOptions{OracleVersion: version, DatabaseRole: databaseRole}, resolved)
}

// ActiveServices returns the actual native cursor for the admitted observation.
func (c *Client) ActiveServices(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.ActiveServicesRows(ctx, c.native(), services.ActiveServicesOptions{OracleVersion: version})
}

// ActiveServicesJSON reads a separate complete bounded cursor.
func (c *Client) ActiveServicesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.ActiveServicesJSON(ctx, c.native(), services.ActiveServicesOptions{OracleVersion: version}, resolved)
}
