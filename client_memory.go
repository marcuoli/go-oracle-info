package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/memory"
)

// RedoBufferAllocation observes exact allocated bytes per visible instance.
// Supply a qualified CDB root or non-CDB handle. The caller owns the returned
// native cursor, iteration, errors, deadline, and Close.
func (c *Client) RedoBufferAllocation(ctx context.Context, filter memory.InstanceFilter) (*sql.Rows, error) {
	options := memory.RedoBufferAllocationOptions{OracleMajor: 9, Filter: filter}
	if _, _, err := memory.RedoBufferAllocationStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return memory.RedoBufferAllocationRows(ctx, c.native(), options)
}

// RedoBufferAllocationJSON consumes a separate cursor using bounded rowset JSON.
// It requires the same admitted scope as RedoBufferAllocation.
func (c *Client) RedoBufferAllocationJSON(ctx context.Context, filter memory.InstanceFilter, limits ...JSONLimits) (json.RawMessage, error) {
	options := memory.RedoBufferAllocationOptions{OracleMajor: 9, Filter: filter}
	if _, _, err := memory.RedoBufferAllocationStatement(options); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return memory.RedoBufferAllocationJSON(ctx, c.native(), options, resolved)
}

// PGAAllocation transfers the native cursor for one executing-instance PGA
// observation. The caller owns validation, iteration, terminal errors and Close.
func (c *Client) PGAAllocation(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return memory.PGAAllocationRows(ctx, c.native(), memory.PGAAllocationOptions{OracleVersion: version})
}

// PGAAllocationJSON reads and validates a separate complete bounded cursor.
func (c *Client) PGAAllocationJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return memory.PGAAllocationJSON(ctx, c.native(), memory.PGAAllocationOptions{OracleVersion: version}, resolved)
}
