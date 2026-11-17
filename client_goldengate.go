package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-info/identity"
)

func (c *Client) goldenGateVersion(ctx context.Context) (string, error) {
	v, err := c.Version(ctx)
	if err != nil {
		return "", err
	}
	if v.State != identity.Available {
		return "", ErrVersionUnavailable
	}
	return v.Value, nil
}

// GoldenGateCapture returns the actual native cursor after observing the source
// version. Callers own iteration/Err/Close, admission and finite stream budgets.
func (c *Client) GoldenGateCapture(ctx context.Context) (*sql.Rows, error) {
	v, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.CaptureRows(ctx, c.native(), v)
}

// GoldenGateCaptureJSON independently consumes the same selection into complete,
// bounded JSON. Failed observations publish no prefix and imply no process health.
func (c *Client) GoldenGateCaptureJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	v, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.CaptureJSON(ctx, c.native(), v, resolved)
}

// GoldenGateApplyCoordinators returns the actual native cursor after observing the source
// version. Callers own iteration/Err/Close, admission and finite stream budgets.
func (c *Client) GoldenGateApplyCoordinators(ctx context.Context) (*sql.Rows, error) {
	v, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.ApplyCoordinatorsRows(ctx, c.native(), v)
}

// GoldenGateApplyCoordinatorsJSON independently consumes the same selection into complete,
// bounded JSON. Failed observations publish no prefix and imply no process health.
func (c *Client) GoldenGateApplyCoordinatorsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	v, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.ApplyCoordinatorsJSON(ctx, c.native(), v, resolved)
}
