package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/instances"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/users"
)

// MemberStartup transfers the actual caller-native cursor.
func (c *Client) MemberStartup(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return instances.MemberStartupRows(ctx, c.native(), instances.MemberStartupOptions{OracleVersion: version})
}

// MemberStartupJSON consumes a separate complete bounded cursor.
func (c *Client) MemberStartupJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return instances.MemberStartupJSON(ctx, c.native(), instances.MemberStartupOptions{OracleVersion: version}, resolved)
}

// PasswordVersionCounts transfers the actual caller-native cursor.
func (c *Client) PasswordVersionCounts(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return users.PasswordVersionCountsRows(ctx, c.native(), users.PasswordVersionCountsOptions{OracleVersion: version})
}

// PasswordVersionCountsJSON consumes a separate complete bounded cursor.
func (c *Client) PasswordVersionCountsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return users.PasswordVersionCountsJSON(ctx, c.native(), users.PasswordVersionCountsOptions{OracleVersion: version}, resolved)
}

// EncryptedColumns transfers the actual caller-native cursor.
func (c *Client) EncryptedColumns(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.EncryptedColumnsRows(ctx, c.native(), objects.EncryptedColumnsOptions{OracleVersion: version})
}

// EncryptedColumnsJSON consumes a separate complete bounded cursor.
func (c *Client) EncryptedColumnsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.EncryptedColumnsJSON(ctx, c.native(), objects.EncryptedColumnsOptions{OracleVersion: version}, resolved)
}
