package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/services"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Scan reads the configured SCAN hostname candidate and remote registration
// evidence through the caller's admitted database handle.
func (c *Client) Scan(ctx context.Context, limits ...JSONLimits) (services.ScanObservation, error) {
	bounds, err := jsonLimits(limits)
	if err != nil {
		return services.ScanObservation{}, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return services.ScanObservation{}, err
	}
	parameterRef, err := reference.NewReference("go-oracle-info", "services", "scan", 1)
	if err != nil {
		return services.ScanObservation{}, err
	}
	listenerRef, err := reference.NewReference("go-oracle-info", "services", "scan", 2)
	if err != nil {
		return services.ScanObservation{}, err
	}
	reader := nativeIdentityReader{
		queryer: c.native(), limits: bounds,
		integerColumns: []string{"INST_ID", "CON_ID", "NUM", "TYPE", "ORDINAL"},
	}
	return services.ReadScan(ctx, reader, services.ScanRequest{
		OracleVersion: version, ParametersReference: parameterRef, ListenersReference: listenerRef, Limits: bounds,
	})
}

// ScanJSON independently encodes the bounded SCAN observation as standard JSON.
func (c *Client) ScanJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	bounds, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	fact, err := c.Scan(ctx, bounds)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	if len(payload) > bounds.MaxBytes {
		return nil, rowset.ErrCapacity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return payload, nil
}

// ListenerNetworks returns a caller-owned native remote registration cursor.
func (c *Client) ListenerNetworks(ctx context.Context) (*sql.Rows, error) {
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.ListenerNetworksRows(ctx, c.native(), services.ListenerNetworksOptions{OracleVersion: version})
}

// ListenerNetworksJSON independently consumes and validates the registration view.
func (c *Client) ListenerNetworksJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	bounds, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.observedNumericVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.ListenerNetworksJSON(ctx, c.native(), services.ListenerNetworksOptions{OracleVersion: version}, bounds)
}
