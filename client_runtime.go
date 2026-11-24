package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/performance"
	"github.com/marcuoli/go-oracle-info/scheduler"
)

// InDoubtTransactions observes the release and transfers the actual native cursor.
func (c *Client) InDoubtTransactions(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return performance.InDoubtTransactionsRows(ctx, c.native(), performance.InDoubtTransactionsOptions{OracleVersion: version})
}

// InDoubtTransactionsJSON consumes a separate complete bounded cursor.
func (c *Client) InDoubtTransactionsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return performance.InDoubtTransactionsJSON(ctx, c.native(), performance.InDoubtTransactionsOptions{OracleVersion: version}, resolved)
}

// RunningJobs observes the release and transfers the actual native cursor.
func (c *Client) RunningJobs(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return scheduler.RunningJobsRows(ctx, c.native(), scheduler.RunningJobsOptions{OracleVersion: version})
}

// RunningJobsJSON consumes a separate complete bounded cursor.
func (c *Client) RunningJobsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return scheduler.RunningJobsJSON(ctx, c.native(), scheduler.RunningJobsOptions{OracleVersion: version}, resolved)
}

// MaterializedViews observes the release and transfers the actual native cursor.
func (c *Client) MaterializedViews(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.MaterializedViewsRows(ctx, c.native(), objects.MaterializedViewsOptions{OracleVersion: version})
}

// MaterializedViewsJSON consumes a separate complete bounded cursor.
func (c *Client) MaterializedViewsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.MaterializedViewsJSON(ctx, c.native(), objects.MaterializedViewsOptions{OracleVersion: version}, resolved)
}

// RecycleBinCounts observes the release and transfers the actual native cursor.
func (c *Client) RecycleBinCounts(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.RecycleBinCountsRows(ctx, c.native(), objects.RecycleBinCountsOptions{OracleVersion: version})
}

// RecycleBinCountsJSON consumes a separate complete bounded cursor.
func (c *Client) RecycleBinCountsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return objects.RecycleBinCountsJSON(ctx, c.native(), objects.RecycleBinCountsOptions{OracleVersion: version}, resolved)
}
