package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/goldengate"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// GoldenGateProcessFilter selects explicit dictionary visibility and exact group
// names. The zero visibility is accessible ALL, never a denied-source fallback.
type GoldenGateProcessFilter struct {
	Visibility goldengate.DictionaryVisibility
	Names      []string
}

func (c *Client) goldenGateProcessOptions(ctx context.Context, f GoldenGateProcessFilter) (goldengate.ProcessOptions, error) {
	if goldengate.ValidateDiscoveryFilter(goldengate.DiscoveryFilter{Extracts: f.Names}) != nil {
		return goldengate.ProcessOptions{}, goldengate.ErrInvalidRequest
	}
	switch f.Visibility {
	case "", goldengate.AccessibleDictionary, goldengate.DatabaseDictionary:
	default:
		return goldengate.ProcessOptions{}, goldengate.ErrInvalidRequest
	}
	version, err := c.goldenGateVersion(ctx)
	return goldengate.ProcessOptions{OracleVersion: version, Visibility: f.Visibility, Names: f.Names}, err
}

// GoldenGateExtractsRows observes the version and returns the actual native
// integrated Extract identity cursor. Callers own Next/Scan/Err/Close and budgets.
func (c *Client) GoldenGateExtractsRows(ctx context.Context, f GoldenGateProcessFilter) (*sql.Rows, error) {
	o, e := c.goldenGateProcessOptions(ctx, f)
	if e != nil {
		return nil, e
	}
	return goldengate.ExtractsRows(ctx, c.native(), o)
}

// GoldenGateExtractsJSON independently returns complete bounded identity JSON.
func (c *Client) GoldenGateExtractsJSON(ctx context.Context, f GoldenGateProcessFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, e := jsonLimits(limits)
	if e != nil {
		return nil, e
	}
	o, e := c.goldenGateProcessOptions(ctx, f)
	if e != nil {
		return nil, e
	}
	return goldengate.ExtractsJSON(ctx, c.native(), o, resolved)
}

// GoldenGateReplicatsRows returns the actual native integrated inbound identity
// cursor. Callers own Next/Scan/Err/Close, source affinity, deadlines and budgets.
func (c *Client) GoldenGateReplicatsRows(ctx context.Context, f GoldenGateProcessFilter) (*sql.Rows, error) {
	o, e := c.goldenGateProcessOptions(ctx, f)
	if e != nil {
		return nil, e
	}
	return goldengate.ReplicatsRows(ctx, c.native(), o)
}

// GoldenGateReplicatsJSON independently returns complete bounded inbound JSON.
func (c *Client) GoldenGateReplicatsJSON(ctx context.Context, f GoldenGateProcessFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, e := jsonLimits(limits)
	if e != nil {
		return nil, e
	}
	o, e := c.goldenGateProcessOptions(ctx, f)
	if e != nil {
		return nil, e
	}
	return goldengate.ReplicatsJSON(ctx, c.native(), o, resolved)
}

func (c *Client) heartbeatCatalogOptions(ctx context.Context, schema string) (goldengate.HeartbeatCatalogOptions, error) {
	if err := goldengate.ValidateDiscoveryFilter(goldengate.DiscoveryFilter{Schema: schema}); err != nil {
		return goldengate.HeartbeatCatalogOptions{}, err
	}
	v, e := c.goldenGateVersion(ctx)
	return goldengate.HeartbeatCatalogOptions{OracleVersion: v, Schema: schema}, e
}

// GoldenGateHeartbeatCatalog returns accessible candidates and verified source
// tokens over the caller's admitted handle. Schema is optional and never guessed.
// Limits bound collection; no connection or continuing object lease is acquired.
func (c *Client) GoldenGateHeartbeatCatalog(ctx context.Context, schema string, limits ...JSONLimits) (goldengate.HeartbeatCatalogResult, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return goldengate.HeartbeatCatalogResult{}, err
	}
	o, err := c.heartbeatCatalogOptions(ctx, schema)
	if err != nil {
		return goldengate.HeartbeatCatalogResult{}, err
	}
	ref, err := reference.NewReference("go-oracle-info", "goldengate", "heartbeat-catalog", 1)
	if err != nil {
		return goldengate.HeartbeatCatalogResult{}, err
	}
	return goldengate.ReadNativeHeartbeatCatalog(ctx, c.native(), goldengate.HeartbeatCatalogRequest{Options: o, Reference: ref}, resolved)
}

// GoldenGateHeartbeatCatalogRows returns the actual metadata cursor. Callers own
// Next/Scan/Err/Close, source affinity, deadlines and finite streaming budgets.
func (c *Client) GoldenGateHeartbeatCatalogRows(ctx context.Context, schema string) (*sql.Rows, error) {
	o, e := c.heartbeatCatalogOptions(ctx, schema)
	if e != nil {
		return nil, e
	}
	return goldengate.HeartbeatCatalogRows(ctx, c.native(), o)
}

// GoldenGateHeartbeatCatalogJSON independently returns complete raw metadata JSON.
func (c *Client) GoldenGateHeartbeatCatalogJSON(ctx context.Context, schema string, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, e := jsonLimits(limits)
	if e != nil {
		return nil, e
	}
	o, e := c.heartbeatCatalogOptions(ctx, schema)
	if e != nil {
		return nil, e
	}
	return goldengate.HeartbeatCatalogJSON(ctx, c.native(), o, resolved)
}

// GoldenGateHeartbeatsRows returns the actual cursor for the verified source.
// The caller preserves its endpoint/container affinity and owns iteration/close.
func (c *Client) GoldenGateHeartbeatsRows(ctx context.Context, o goldengate.HeartbeatsOptions) (*sql.Rows, error) {
	return goldengate.HeartbeatsRows(ctx, c.native(), o)
}

// GoldenGateHeartbeatsJSON consumes a separate complete bounded heartbeat cursor.
// Source tokens retain the version/structure observed in their admitted catalog.
func (c *Client) GoldenGateHeartbeatsJSON(ctx context.Context, o goldengate.HeartbeatsOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, e := jsonLimits(limits)
	if e != nil {
		return nil, e
	}
	return goldengate.HeartbeatsJSON(ctx, c.native(), o, resolved)
}

// GoldenGateHeartbeatLagRows returns the actual verified-view cursor; callers
// own iteration/Err/Close, endpoint/container affinity, deadlines and budgets.
func (c *Client) GoldenGateHeartbeatLagRows(ctx context.Context, o goldengate.HeartbeatLagOptions) (*sql.Rows, error) {
	return goldengate.HeartbeatLagRows(ctx, c.native(), o)
}

// GoldenGateHeartbeatLagJSON independently returns exact lag/age text or no bytes.
func (c *Client) GoldenGateHeartbeatLagJSON(ctx context.Context, o goldengate.HeartbeatLagOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, e := jsonLimits(limits)
	if e != nil {
		return nil, e
	}
	return goldengate.HeartbeatLagJSON(ctx, c.native(), o, resolved)
}
