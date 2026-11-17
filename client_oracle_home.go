package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/rowset"
)

// OracleHome reads the executing instance's environment on 9i+ or an explicitly
// inferred dictionary-library candidate after qualified source failures. It
// observes version afresh and owns no connection/host access; cancellation and
// unknown/retired-session errors stop the chain.
func (c *Client) OracleHome(ctx context.Context) (identity.OracleHomeObservation, error) {
	major, err := c.major(ctx)
	if err != nil {
		return identity.OracleHomeObservation{}, err
	}
	ref, err := propertyReference("oracle_home", 1)
	if err != nil {
		return identity.OracleHomeObservation{}, err
	}
	return identity.ReadOracleHome(ctx, nativeIdentityReader{queryer: c.native()}, identity.OracleHomeRequest{OracleMajor: major, Reference: ref})
}

// OracleHomeRows returns the actual caller-native selected-home cursor. The
// caller owns Next/Scan/Err/Close, budgets and executing-instance affinity.
func (c *Client) OracleHomeRows(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.OracleHomeRows(ctx, c.native(), major)
}

// OracleHomeRowsJSON independently returns complete bounded source rows with
// provenance/inference, retaining SQL NULL and exact native member numbers.
func (c *Client) OracleHomeRowsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.OracleHomeRowsJSON(ctx, c.native(), major, resolved)
}

// OracleHomeJSON independently reads the fallback property and encodes the full
// validated observation/attempts. No driver error text or failed prefix is returned.
func (c *Client) OracleHomeJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if resolved.MaxBytes < 2 {
		return nil, rowset.ErrCapacity
	}
	fact, err := c.OracleHome(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	if len(payload) > resolved.MaxBytes {
		return nil, rowset.ErrCapacity
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return payload, nil
}
