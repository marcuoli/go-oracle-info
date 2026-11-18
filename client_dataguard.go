package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/dataguard"
	"github.com/marcuoli/go-oracle-info/identity"
)

// DataGuardFilter selects visible reporting IDs, without complete RAC inference.
type DataGuardFilter = dataguard.Filter

// DataGuardArchiveProgressOptions admits paired incarnation identity, not a past-date window.
type DataGuardArchiveProgressOptions = dataguard.ArchiveProgressOptions

func (c *Client) dataGuardVersion(ctx context.Context) (string, error) {
	observation, err := c.Version(ctx)
	if err != nil {
		return "", err
	}
	if observation.State != identity.Available {
		return "", ErrVersionUnavailable
	}
	return observation.Value, nil
}

// DataGuardDatabaseContext observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardDatabaseContext(ctx context.Context) (*sql.Rows, error) {
	if _, _, err := dataguard.DatabaseContextStatement("19.0.0.0.0"); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.DatabaseContextRows(ctx, c.native(), version)
}

// DataGuardDatabaseContextJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardDatabaseContextJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.DatabaseContextStatement("19.0.0.0.0"); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.DatabaseContextJSON(ctx, c.native(), version, resolved)
}

// DataGuardStandbyLogs observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardStandbyLogs(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.StandbyLogsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.StandbyLogsRows(ctx, c.native(), version, filter)
}

// DataGuardStandbyLogsJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardStandbyLogsJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.StandbyLogsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.StandbyLogsJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardProcesses observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardProcesses(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.ProcessesStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ProcessesRows(ctx, c.native(), version, filter)
}

// DataGuardProcessesJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardProcessesJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.ProcessesStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ProcessesJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardRecoveryProgress observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardRecoveryProgress(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.RecoveryProgressStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.RecoveryProgressRows(ctx, c.native(), version, filter)
}

// DataGuardRecoveryProgressJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardRecoveryProgressJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.RecoveryProgressStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.RecoveryProgressJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardArchiveGaps observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardArchiveGaps(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.ArchiveGapsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveGapsRows(ctx, c.native(), version, filter)
}

// DataGuardArchiveGapsJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardArchiveGapsJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.ArchiveGapsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveGapsJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardArchiveDestinations observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardArchiveDestinations(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.ArchiveDestinationsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveDestinationsRows(ctx, c.native(), version, filter)
}

// DataGuardArchiveDestinationsJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardArchiveDestinationsJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.ArchiveDestinationsStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveDestinationsJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardArchiveDestinationStatus observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardArchiveDestinationStatus(ctx context.Context, filter DataGuardFilter) (*sql.Rows, error) {
	if _, _, err := dataguard.ArchiveDestinationStatusStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveDestinationStatusRows(ctx, c.native(), version, filter)
}

// DataGuardArchiveDestinationStatusJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardArchiveDestinationStatusJSON(ctx context.Context, filter DataGuardFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.ArchiveDestinationStatusStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveDestinationStatusJSON(ctx, c.native(), version, filter, resolved)
}

// DataGuardArchiveProgress observes the source version and returns the caller-native cursor.
// Callers own iteration, Err, Close and budgets; observations are not atomic across calls.
func (c *Client) DataGuardArchiveProgress(ctx context.Context, filter DataGuardArchiveProgressOptions) (*sql.Rows, error) {
	if _, _, err := dataguard.ArchiveProgressStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveProgressRows(ctx, c.native(), version, filter)
}

// DataGuardArchiveProgressJSON consumes its separate cursor through the shared finite rowset path.
// No JSON prefix is returned on query, terminal, decode, close, cancellation or budget failure.
func (c *Client) DataGuardArchiveProgressJSON(ctx context.Context, filter DataGuardArchiveProgressOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err := dataguard.ArchiveProgressStatement("19.0.0.0.0", filter); err != nil {
		return nil, err
	}
	version, err := c.dataGuardVersion(ctx)
	if err != nil {
		return nil, err
	}
	return dataguard.ArchiveProgressJSON(ctx, c.native(), version, filter, resolved)
}
