package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/asm"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/redo"
	"github.com/marcuoli/go-oracle-info/storage"
)

// RedoFilter selects qualified visible reporting IDs without RAC-total inference.
type RedoFilter = redo.Filter

// RedoHistoryFilter admits finite database-local [From,Until) history boundaries.
type RedoHistoryFilter = redo.HistoryFilter

// TempfileOptions explicitly admits optional SHARED and owning INST_ID columns.
type TempfileOptions struct{ IncludeLocal bool }

// SGAComponentFilter selects reporting IDs for observed allocation components.
type SGAComponentFilter = storage.InstanceFilter

// ASMClientFilter requires the caller-observed database or ASM source role.
type ASMClientFilter = asm.Filter

// RedoGroups observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) RedoGroups(ctx context.Context, filter RedoFilter) (*sql.Rows, error) {
	if _, _, err := redo.GroupsStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.GroupsRows(ctx, c.native(), major, filter)
}

// RedoGroupsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) RedoGroupsJSON(ctx context.Context, filter RedoFilter, limits ...JSONLimits) (json.RawMessage, error) {
	if _, _, err := redo.GroupsStatement(11, filter); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.GroupsJSON(ctx, c.native(), major, filter, resolved)
}

// RedoMembers observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) RedoMembers(ctx context.Context, filter RedoFilter) (*sql.Rows, error) {
	if _, _, err := redo.MembersStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.MembersRows(ctx, c.native(), major, filter)
}

// RedoMembersJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) RedoMembersJSON(ctx context.Context, filter RedoFilter, limits ...JSONLimits) (json.RawMessage, error) {
	if _, _, err := redo.MembersStatement(11, filter); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.MembersJSON(ctx, c.native(), major, filter, resolved)
}

// RedoHistory observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) RedoHistory(ctx context.Context, filter RedoHistoryFilter) (*sql.Rows, error) {
	if _, _, err := redo.HistoryStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.HistoryRows(ctx, c.native(), major, filter)
}

// RedoHistoryJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) RedoHistoryJSON(ctx context.Context, filter RedoHistoryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	if _, _, err := redo.HistoryStatement(11, filter); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.HistoryJSON(ctx, c.native(), major, filter, resolved)
}

// Tablespaces observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) Tablespaces(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TablespacesRows(ctx, c.native(), major)
}

// TablespacesJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) TablespacesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TablespacesJSON(ctx, c.native(), major, resolved)
}

// Datafiles observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) Datafiles(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DatafilesRows(ctx, c.native(), major)
}

// DatafilesJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) DatafilesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DatafilesJSON(ctx, c.native(), major, resolved)
}

// FreeExtents observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) FreeExtents(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.FreeExtentsRows(ctx, c.native(), major)
}

// FreeExtentsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) FreeExtentsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.FreeExtentsJSON(ctx, c.native(), major, resolved)
}

// Segments observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) Segments(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.SegmentsRows(ctx, c.native(), major)
}

// SegmentsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) SegmentsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.SegmentsJSON(ctx, c.native(), major, resolved)
}

// LongColumns observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) LongColumns(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.LongColumnsRows(ctx, c.native(), major)
}

// LongColumnsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) LongColumnsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.LongColumnsJSON(ctx, c.native(), major, resolved)
}

// DefaultTemporary observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) DefaultTemporary(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DefaultTemporaryRows(ctx, c.native(), major)
}

// DefaultTemporaryJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) DefaultTemporaryJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DefaultTemporaryJSON(ctx, c.native(), major, resolved)
}

// Tempfiles observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) Tempfiles(ctx context.Context, options TempfileOptions) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TempfilesRows(ctx, c.native(), storage.TempfileOptions{OracleMajor: major, IncludeLocal: options.IncludeLocal})
}

// TempfilesJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) TempfilesJSON(ctx context.Context, options TempfileOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TempfilesJSON(ctx, c.native(), storage.TempfileOptions{OracleMajor: major, IncludeLocal: options.IncludeLocal}, resolved)
}

// SGAComponents observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) SGAComponents(ctx context.Context, filter SGAComponentFilter) (*sql.Rows, error) {
	if _, _, err := storage.SGAComponentsStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.SGAComponentsRows(ctx, c.native(), major, filter)
}

// SGAComponentsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) SGAComponentsJSON(ctx context.Context, filter SGAComponentFilter, limits ...JSONLimits) (json.RawMessage, error) {
	if _, _, err := storage.SGAComponentsStatement(11, filter); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.SGAComponentsJSON(ctx, c.native(), major, filter, resolved)
}

// ASMClients observes the source version then returns the actual native cursor.
// Callers retain iteration, Err, Close, deadlines, admission and finite budgets.
func (c *Client) ASMClients(ctx context.Context, filter ASMClientFilter) (*sql.Rows, error) {
	if _, _, err := asm.ClientsStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return asm.ClientsRows(ctx, c.native(), major, filter)
}

// ASMClientsJSON independently consumes the same selection through shared
// bounded JSON ownership. No prefix is returned on any failure.
func (c *Client) ASMClientsJSON(ctx context.Context, filter ASMClientFilter, limits ...JSONLimits) (json.RawMessage, error) {
	if _, _, err := asm.ClientsStatement(11, filter); err != nil {
		return nil, err
	}
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return asm.ClientsJSON(ctx, c.native(), major, filter, resolved)
}

// TemporaryGroups returns raw membership for a default temporary group property.
// The caller owns iteration, Err, Close and finite stream budgets.
func (c *Client) TemporaryGroups(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TemporaryGroupsRows(ctx, c.native(), major)
}

// TemporaryGroupsJSON independently consumes complete bounded group membership.
func (c *Client) TemporaryGroupsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TemporaryGroupsJSON(ctx, c.native(), major, resolved)
}

// ArchivedLogs returns raw archive copies in the admitted local first-time range.
// Archive allocation is separate from historical generated-redo estimates.
func (c *Client) ArchivedLogs(ctx context.Context, filter RedoHistoryFilter) (*sql.Rows, error) {
	if _, _, err := redo.ArchivedLogsStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ArchivedLogsRows(ctx, c.native(), major, filter)
}

// ArchivedLogsJSON independently reads bounded archive-copy evidence.
func (c *Client) ArchivedLogsJSON(ctx context.Context, filter RedoHistoryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err = redo.ArchivedLogsStatement(11, filter); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return redo.ArchivedLogsJSON(ctx, c.native(), major, filter, resolved)
}

// TempFreeSpace observes the release and returns the actual TEMP free-space
// cursor. The caller owns iteration, Err, Close and finite stream budgets.
func (c *Client) TempFreeSpace(ctx context.Context) (*sql.Rows, error) {
	version, err := c.storageVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TempFreeSpaceRows(ctx, c.native(), version)
}

// TempFreeSpaceJSON returns one complete bounded TEMP free-space dataset.
func (c *Client) TempFreeSpaceJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.storageVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.TempFreeSpaceJSON(ctx, c.native(), version, resolved)
}

// BlockCorruptions observes the release and returns the actual cursor over
// recorded ranges. The caller owns iteration, Err, Close and its read budget.
func (c *Client) BlockCorruptions(ctx context.Context) (*sql.Rows, error) {
	version, err := c.storageVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.BlockCorruptionsRows(ctx, c.native(), version)
}

// BlockCorruptionsJSON returns one bounded dataset of recorded corruption.
func (c *Client) BlockCorruptionsJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	version, err := c.storageVersion(ctx)
	if err != nil {
		return nil, err
	}
	return storage.BlockCorruptionsJSON(ctx, c.native(), version, resolved)
}

func (c *Client) storageVersion(ctx context.Context) (string, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return "", err
	}
	if version.State != identity.Available {
		return "", ErrVersionUnavailable
	}
	return version.Value, nil
}
