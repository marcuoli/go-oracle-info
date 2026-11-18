package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// GoldenGate reads database-visible administrator properties. ConfiguredSchema
// remains unavailable: privilege grantees do not establish external GGSCHEMA.
// Calls are independent observations over the caller's admitted handle/scope.
func (c *Client) GoldenGate(ctx context.Context) (goldengate.Properties, error) {
	return c.goldenGateProperties(ctx, rowset.Limits{})
}
func (c *Client) goldenGateProperties(ctx context.Context, limits rowset.Limits) (goldengate.Properties, error) {
	version, err := c.goldenGateVersion(ctx)
	if err != nil {
		return goldengate.Properties{}, err
	}
	ref, err := reference.NewReference("go-oracle-info", "goldengate", "administrators", 1)
	if err != nil {
		return goldengate.Properties{}, err
	}
	return goldengate.ReadNativeProperties(ctx, c.native(), goldengate.Request{OracleVersion: version, Reference: ref}, limits)
}

// GoldenGateJSON independently encodes the complete typed properties. Limits
// bound native collection and the final JSON; no failed prefix is published.
func (c *Client) GoldenGateJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	facts, err := c.goldenGateProperties(ctx, resolved)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	if len(data) > resolved.MaxBytes {
		return nil, rowset.ErrCapacity
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// GoldenGateAdministratorsRows returns the actual native administrator cursor.
// Caller owns Next/Scan/Err/Close, affinity, deadlines and finite stream budgets.
func (c *Client) GoldenGateAdministratorsRows(ctx context.Context) (*sql.Rows, error) {
	version, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.AdministratorsRows(ctx, c.native(), version)
}

// GoldenGateAdministratorsJSON independently returns complete selected native
// privilege rows with provenance, exact identifiers and projected SQL NULLs.
func (c *Client) GoldenGateAdministratorsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.goldenGateVersion(ctx)
	if err != nil {
		return nil, err
	}
	return goldengate.AdministratorsJSON(ctx, c.native(), version, resolved)
}
