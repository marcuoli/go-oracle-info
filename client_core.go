package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/patches"
	"github.com/marcuoli/go-oracle-info/registry"
	"github.com/marcuoli/go-oracle-info/services"
)

// DatabaseInfoOptions explicitly admits the optional procedural-supplemental
// column. The facade independently observes the Oracle release on every call.
type DatabaseInfoOptions struct{ IncludeSupplementalPL bool }

// PatchHistoryOptions explicitly admits BUNDLE_SERIES on this source.
type PatchHistoryOptions struct{ IncludeBundleSeries bool }

// DatabaseNLS returns the caller-native cursor for permanent database NLS parameters.
// Callers own iteration/Err/Close and budgets; no connection ownership transfers.
func (c *Client) DatabaseNLS(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return database.NLSDatabaseRows(ctx, c.native(), major)
}

// DatabaseNLSJSON independently consumes DatabaseNLS's source into bounded complete JSON.
// No prefix survives terminal, close, cancellation or capacity failure.
func (c *Client) DatabaseNLSJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return database.NLSDatabaseJSON(ctx, c.native(), major, resolved)
}

// DatabaseOptions returns the caller-native cursor for installed option/feature flags, without licensing inference.
// Callers own iteration/Err/Close and budgets; no connection ownership transfers.
func (c *Client) DatabaseOptions(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return database.OptionsRows(ctx, c.native(), major)
}

// DatabaseOptionsJSON independently consumes DatabaseOptions's source into bounded complete JSON.
// No prefix survives terminal, close, cancellation or capacity failure.
func (c *Client) DatabaseOptionsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return database.OptionsJSON(ctx, c.native(), major, resolved)
}

// Components returns the caller-native cursor for dictionary components, retaining full version and raw modified text.
// Callers own iteration/Err/Close and budgets; no connection ownership transfers.
func (c *Client) Components(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return registry.ComponentsRows(ctx, c.native(), registry.ComponentsOptions{OracleMajor: major})
}

// ComponentsJSON independently consumes Components's source into bounded complete JSON.
// No prefix survives terminal, close, cancellation or capacity failure.
func (c *Client) ComponentsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return registry.ComponentsJSON(ctx, c.native(), registry.ComponentsOptions{OracleMajor: major}, resolved)
}

// Services returns the caller-native cursor for detailed dictionary service metadata, without reachability inference.
// Callers own iteration/Err/Close and budgets; no connection ownership transfers.
func (c *Client) Services(ctx context.Context) (*sql.Rows, error) {
	major, includeEdition, err := c.serviceVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.Rows(ctx, c.native(), major, includeEdition)
}

// ServicesJSON independently consumes Services' version-selected projection.
func (c *Client) ServicesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, includeEdition, err := c.serviceVersion(ctx)
	if err != nil {
		return nil, err
	}
	return services.JSON(ctx, c.native(), major, resolved, includeEdition)
}

func (c *Client) serviceVersion(ctx context.Context) (int, bool, error) {
	observed, err := c.Version(ctx)
	if err != nil {
		return 0, false, err
	}
	if observed.State != identity.Available {
		return 0, false, ErrVersionUnavailable
	}
	major, err := identity.Major(observed.Value)
	if err != nil {
		return 0, false, err
	}
	if major != 11 {
		return major, major >= 12, nil
	}
	parts := strings.Split(observed.Value, ".")
	if len(parts) != 5 {
		return 0, false, ErrVersionUnavailable
	}
	minor, minorErr := strconv.Atoi(parts[1])
	update, updateErr := strconv.Atoi(parts[2])
	patch, patchErr := strconv.Atoi(parts[3])
	if minorErr != nil || updateErr != nil || patchErr != nil {
		return 0, false, ErrVersionUnavailable
	}
	return major, minor > 2 || (minor == 2 && (update > 0 || patch >= 2)), nil
}

// SessionContext returns the caller-native cursor for observed executing-session client host, OS user and database user.
// Callers own iteration/Err/Close and budgets; no connection ownership transfers.
func (c *Client) SessionContext(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.SessionContextRows(ctx, c.native(), major)
}

// SessionContextJSON independently consumes SessionContext's source into bounded complete JSON.
// No prefix survives terminal, close, cancellation or capacity failure.
func (c *Client) SessionContextJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.SessionContextJSON(ctx, c.native(), major, resolved)
}

// DatabaseInfo returns raw controlfile/logging/SCN properties in a caller-owned
// native cursor. IncludeSupplementalPL is an explicit source-capability admission.
func (c *Client) DatabaseInfo(ctx context.Context, options DatabaseInfoOptions) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.DatabaseInfoRows(ctx, c.native(), identity.DatabaseInfoOptions{OracleMajor: major, IncludeSupplementalPL: options.IncludeSupplementalPL})
}

// DatabaseInfoJSON independently consumes the same admitted database projection.
func (c *Client) DatabaseInfoJSON(ctx context.Context, options DatabaseInfoOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return identity.DatabaseInfoJSON(ctx, c.native(), identity.DatabaseInfoOptions{OracleMajor: major, IncludeSupplementalPL: options.IncludeSupplementalPL}, resolved)
}

// InvalidObjects returns invalid-object identity/status and creation/last-DDL
// civil labels. Empty owners means all dictionary-visible owners, with no blacklist.
func (c *Client) InvalidObjects(ctx context.Context, owners ...string) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return objects.InvalidDetailsRows(ctx, c.native(), major, owners...)
}

// InvalidObjectsJSON independently consumes the same exact bound owner selection.
func (c *Client) InvalidObjectsJSON(ctx context.Context, owners []string, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return objects.InvalidDetailsJSON(ctx, c.native(), major, owners, resolved)
}

// PatchHistory returns documented upgrade/downgrade/patch history. It never reads
// REGISTRY$HISTORY directly and remains distinct from datapatch attempts.
func (c *Client) PatchHistory(ctx context.Context, options PatchHistoryOptions) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return patches.HistoryRows(ctx, c.native(), patches.HistoryOptions{OracleMajor: major, IncludeBundleSeries: options.IncludeBundleSeries})
}

// PatchHistoryJSON independently consumes the documented history projection.
func (c *Client) PatchHistoryJSON(ctx context.Context, options PatchHistoryOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return patches.HistoryJSON(ctx, c.native(), patches.HistoryOptions{OracleMajor: major, IncludeBundleSeries: options.IncludeBundleSeries}, resolved)
}

func (c *Client) sqlPatchOptions(ctx context.Context) (patches.SQLPatchOptions, error) {
	observed, err := c.Version(ctx)
	if err != nil {
		return patches.SQLPatchOptions{}, err
	}
	if observed.State != identity.Available {
		return patches.SQLPatchOptions{}, ErrVersionUnavailable
	}
	major, err := identity.Major(observed.Value)
	if err != nil {
		return patches.SQLPatchOptions{}, err
	}
	options := patches.SQLPatchOptions{OracleMajor: major}
	if major == 12 {
		parts := strings.Split(observed.Value, ".")
		if len(parts) < 2 {
			return patches.SQLPatchOptions{}, ErrVersionUnavailable
		}
		options.OracleMinor, err = strconv.Atoi(parts[1])
		if err != nil {
			return patches.SQLPatchOptions{}, ErrVersionUnavailable
		}
	}
	return options, nil
}

// SQLPatches returns datapatch installation attempts. It observes the release to
// select the documented 12.1/12.2/18+ metadata layout without probing failed SQL.
func (c *Client) SQLPatches(ctx context.Context) (*sql.Rows, error) {
	options, err := c.sqlPatchOptions(ctx)
	if err != nil {
		return nil, err
	}
	return patches.SQLPatchRows(ctx, c.native(), options)
}

// SQLPatchesJSON independently consumes the same version-selected attempt source.
func (c *Client) SQLPatchesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options, err := c.sqlPatchOptions(ctx)
	if err != nil {
		return nil, err
	}
	return patches.SQLPatchJSON(ctx, c.native(), options, resolved)
}
