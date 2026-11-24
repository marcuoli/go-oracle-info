package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/containers"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
)

// GlobalName transfers the actual native cursor for the admitted dictionary.
func (c *Client) GlobalName(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return identity.GlobalNameRows(ctx, c.native(), identity.GlobalNameOptions{OracleVersion: version})
}

// GlobalNameJSON reads a separate complete bounded cursor.
func (c *Client) GlobalNameJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return identity.GlobalNameJSON(ctx, c.native(), identity.GlobalNameOptions{OracleVersion: version}, resolved)
}

// DatabaseProperties transfers the actual native cursor for the admitted dictionary.
func (c *Client) DatabaseProperties(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.PropertiesRows(ctx, c.native(), database.PropertiesOptions{OracleVersion: version})
}

// DatabasePropertiesJSON reads a separate complete bounded cursor.
func (c *Client) DatabasePropertiesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.PropertiesJSON(ctx, c.native(), database.PropertiesOptions{OracleVersion: version}, resolved)
}

// CDBRegistryComponents transfers the actual native cursor for the admitted dictionary.
func (c *Client) CDBRegistryComponents(ctx context.Context, scope containers.Scope) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.RegistryComponentsRows(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope})
}

// CDBRegistryComponentsJSON reads a separate complete bounded cursor.
func (c *Client) CDBRegistryComponentsJSON(ctx context.Context, scope containers.Scope, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.RegistryComponentsJSON(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope}, resolved)
}

// CDBInvalidObjectCounts transfers the actual native cursor for the admitted dictionary.
func (c *Client) CDBInvalidObjectCounts(ctx context.Context, scope containers.Scope) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.InvalidObjectCountsRows(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope})
}

// CDBInvalidObjectCountsJSON reads a separate complete bounded cursor.
func (c *Client) CDBInvalidObjectCountsJSON(ctx context.Context, scope containers.Scope, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.InvalidObjectCountsJSON(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope}, resolved)
}

// CDBSQLPatchHistory transfers the actual native cursor for the admitted dictionary.
func (c *Client) CDBSQLPatchHistory(ctx context.Context, scope containers.Scope) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.SQLPatchHistoryRows(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope})
}

// CDBSQLPatchHistoryJSON reads a separate complete bounded cursor.
func (c *Client) CDBSQLPatchHistoryJSON(ctx context.Context, scope containers.Scope, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.SQLPatchHistoryJSON(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope}, resolved)
}

// CDBLegacyAuditTablePlacement returns the native cursor for the admitted root.
func (c *Client) CDBLegacyAuditTablePlacement(ctx context.Context, scope containers.Scope) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.LegacyAuditTablePlacementRows(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope})
}

// CDBLegacyAuditTablePlacementJSON consumes a separate complete bounded cursor.
func (c *Client) CDBLegacyAuditTablePlacementJSON(ctx context.Context, scope containers.Scope, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return containers.LegacyAuditTablePlacementJSON(ctx, c.native(), containers.RootDictionaryOptions{OracleVersion: version, Scope: scope}, resolved)
}
