package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/backup"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/dataguard"
	"github.com/marcuoli/go-oracle-info/storage"
)

// RecoveryArea observes the release and transfers the actual native cursor.
func (c *Client) RecoveryArea(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.RecoveryAreaRows(ctx, c.native(), storage.RecoveryAreaOptions{OracleVersion: version})
}

// RecoveryAreaJSON consumes a separate complete bounded cursor.
func (c *Client) RecoveryAreaJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.RecoveryAreaJSON(ctx, c.native(), storage.RecoveryAreaOptions{OracleVersion: version}, resolved)
}

// RecoveryAreaUsage observes the release and transfers the actual native cursor.
func (c *Client) RecoveryAreaUsage(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.RecoveryAreaUsageRows(ctx, c.native(), storage.RecoveryAreaOptions{OracleVersion: version})
}

// RecoveryAreaUsageJSON consumes a separate complete bounded cursor.
func (c *Client) RecoveryAreaUsageJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.RecoveryAreaUsageJSON(ctx, c.native(), storage.RecoveryAreaOptions{OracleVersion: version}, resolved)
}

// RestorePoints observes the release and transfers the actual native cursor.
func (c *Client) RestorePoints(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return backup.RestorePointsRows(ctx, c.native(), backup.RestorePointsOptions{OracleVersion: version})
}

// RestorePointsJSON consumes a separate complete bounded cursor.
func (c *Client) RestorePointsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return backup.RestorePointsJSON(ctx, c.native(), backup.RestorePointsOptions{OracleVersion: version}, resolved)
}

// DatabaseIncarnations observes the release and transfers the actual native cursor.
func (c *Client) DatabaseIncarnations(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.DatabaseIncarnationsRows(ctx, c.native(), version)
}

// DatabaseIncarnationsJSON consumes a separate complete bounded cursor.
func (c *Client) DatabaseIncarnationsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.DatabaseIncarnationsJSON(ctx, c.native(), version, resolved)
}

// StandbyPromotionSCN observes the release and transfers the actual native cursor.
func (c *Client) StandbyPromotionSCN(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.StandbyPromotionSCNRows(ctx, c.native(), version)
}

// StandbyPromotionSCNJSON consumes a separate complete bounded cursor.
func (c *Client) StandbyPromotionSCNJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.StandbyPromotionSCNJSON(ctx, c.native(), version, resolved)
}

// SessionClock observes the release and transfers the connected session's native clock cursor.
func (c *Client) SessionClock(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.SessionClockRows(ctx, c.native(), database.SessionClockOptions{OracleVersion: version})
}

// SessionClockJSON consumes a separate complete bounded clock cursor.
func (c *Client) SessionClockJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.SessionClockJSON(ctx, c.native(), database.SessionClockOptions{OracleVersion: version}, resolved)
}

// TimeZone observes the release and transfers the connected endpoint's native DBTIMEZONE cursor.
func (c *Client) TimeZone(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.TimeZoneRows(ctx, c.native(), database.TimeZoneOptions{OracleVersion: version})
}

// TimeZoneJSON consumes a separate complete bounded cursor.
func (c *Client) TimeZoneJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.TimeZoneJSON(ctx, c.native(), database.TimeZoneOptions{OracleVersion: version}, resolved)
}

// TimeZoneFiles observes the release and transfers the actual native cursor.
func (c *Client) TimeZoneFiles(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.TimeZoneFilesRows(ctx, c.native(), database.TimeZoneFilesOptions{OracleVersion: version})
}

// TimeZoneFilesJSON consumes a separate complete bounded cursor.
func (c *Client) TimeZoneFilesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return database.TimeZoneFilesJSON(ctx, c.native(), database.TimeZoneFilesOptions{OracleVersion: version}, resolved)
}
