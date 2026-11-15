package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/parameters"
)

// HiddenContextFilter selects raw internal context values independently of the
// leading-underscore catalog. Zero retains default and ordinary-name rows too.
type HiddenContextFilter = parameters.HiddenContextFilter

// HiddenParameterContext returns the actual caller-native X$KSPPCV2 cursor after
// observing the source version. The caller owns Next/Scan/Err/Close, finite stream
// budgets and connected-member affinity. It proves no all-member RAC coverage.
func (c *Client) HiddenParameterContext(ctx context.Context, filter HiddenContextFilter) (*sql.Rows, error) {
	if _, _, err := parameters.HiddenContextStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenContextRows(ctx, c.native(), major, filter)
}

// HiddenParameterContextJSON independently captures the same complete context
// selection as bounded JSON. Default/name admission matches the typed decoder;
// the caller records unavailable and missing members separately from empty rows.
func (c *Client) HiddenParameterContextJSON(ctx context.Context, filter HiddenContextFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err = parameters.HiddenContextStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.HiddenContextJSON(ctx, c.native(), major, filter, resolved)
}
