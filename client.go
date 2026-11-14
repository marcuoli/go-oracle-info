// Package oracleinfo provides non-owning fact properties and native row/JSON
// datasets over a caller-admitted database/sql QueryContext handle.
package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/parameters"
	"github.com/marcuoli/go-oracle-info/rowset"
)

// Queryer is a caller-owned native database/sql handle. Prefer an admitted,
// session-affine *sql.Conn or *sql.Tx; a pool requires caller-proven compatible
// targets and cursor concurrency. The facade acquires no connection or lease.
type Queryer = rowset.Queryer

// ParameterFilter selects exact names and reporting INST_ID values.
type ParameterFilter = parameters.Filter

// SPFileFilter keeps reporting INST_ID separate from stored SID clauses.
type SPFileFilter = parameters.SPFileFilter

// JSONLimits selects positive row/encoded-byte bounds; zero uses safe defaults.
type JSONLimits = rowset.Limits

// Client borrows its handle without owning its lifetime or caching capabilities.
// Independent property calls are independent observations, not an atomic snapshot.
type Client struct{ queryer Queryer }

// New stores an already-admitted handle. It opens/closes/reconfigures nothing;
// method calls report invalid handles. Close the native handle in its owner.
func New(queryer Queryer) *Client { return &Client{queryer: queryer} }

// ErrVersionUnavailable prevents a dataset read without usable release evidence.
var ErrVersionUnavailable = errors.New("Oracle version unavailable for parameter selection")

func (c *Client) native() Queryer {
	if c == nil {
		return nil
	}
	return c.queryer
}

// Version independently reads typed numeric version/precision/source evidence.
// A returned error remains significant even if a legacy value is retained.
func (c *Client) Version(ctx context.Context) (identity.VersionObservation, error) {
	refs, err := versionReferences()
	if err != nil {
		return identity.VersionObservation{}, err
	}
	return identity.ReadVersion(ctx, nativeIdentityReader{queryer: c.native()}, refs)
}

// Architecture distinguishes the connected container from its parent CDB flag.
func (c *Client) Architecture(ctx context.Context) (identity.ArchitectureObservation, error) {
	refs, err := architectureReferences()
	if err != nil {
		return identity.ArchitectureObservation{}, err
	}
	return identity.ReadArchitecture(ctx, nativeIdentityReader{queryer: c.native()}, refs)
}

// Edition preserves explicit edition code/label provenance without licensing inference.
func (c *Client) Edition(ctx context.Context) (identity.EditionObservation, error) {
	refs, err := editionReferences()
	if err != nil {
		return identity.EditionObservation{}, err
	}
	return identity.ReadEdition(ctx, nativeIdentityReader{queryer: c.native()}, refs)
}

func (c *Client) major(ctx context.Context) (int, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return 0, err
	}
	if version.State != identity.Available {
		return 0, ErrVersionUnavailable
	}
	return identity.Major(version.Value)
}

// InitParameters returns the actual caller-native cursor of selected effective
// instance values. It creates no return-time cancellation or SQL row truncation.
// Callers own iteration, Err, Close and budgets; use JSON for bounded consumption.
func (c *Client) InitParameters(ctx context.Context, filter ParameterFilter) (*sql.Rows, error) {
	if _, _, err := parameters.EffectiveStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.EffectiveRows(ctx, c.native(), major, filter)
}

// SPFileParameters returns raw stored clauses, never inferred effective values.
// Instances are reporting IDs; named SIDs preserve wildcard defaults as well.
func (c *Client) SPFileParameters(ctx context.Context, filter SPFileFilter) (*sql.Rows, error) {
	if _, _, err := parameters.SPFileStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.SPFileRows(ctx, c.native(), major, filter)
}

// InitParametersJSON executes the same selection as InitParameters using its own
// cursor and standard JSON. Zero/one limits value is accepted; no prefix is returned.
func (c *Client) InitParametersJSON(ctx context.Context, filter ParameterFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := parameters.EffectiveStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.EffectiveJSON(ctx, c.native(), major, filter, resolved)
}

// SPFileParametersJSON independently consumes/finalizes SPFileParameters' same
// statement/filter. Numeric reporting IDs/ordinals and textual SPFILE TYPE differ.
func (c *Client) SPFileParametersJSON(ctx context.Context, filter SPFileFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := parameters.SPFileStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return parameters.SPFileJSON(ctx, c.native(), major, filter, resolved)
}

func jsonLimits(limits []JSONLimits) (JSONLimits, error) {
	if len(limits) > 1 {
		return JSONLimits{}, rowset.ErrInvalidRequest
	}
	if len(limits) == 0 {
		return rowset.Resolve(JSONLimits{})
	}
	return rowset.Resolve(limits[0])
}
