package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/redo"
)

// RedoClock observes the version then returns the connected-source SYSDATE
// native cursor. The caller owns iteration, one-row admission, Err/Close and
// finite budgets; the civil label implies no timezone or cross-member clock sync.
func (c *Client) RedoClock(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ClockRows(ctx, c.native(), major)
}

// RedoClockJSON independently reads the source-local civil clock with complete
// bounded domain admission. Caller collection UTC is a separate observation.
func (c *Client) RedoClockJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ClockJSON(ctx, c.native(), major, resolved)
}
