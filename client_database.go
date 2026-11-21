package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/storage"
)

// ErrArchitectureUnavailable rejects an unqualified connected-container scope;
// callers must not substitute CDB-wide data for an unknown or direct PDB source.
var ErrArchitectureUnavailable = errors.New("Oracle connected architecture unavailable")

// DatabaseFiles returns the actual native controlfile data/temp-file cursor.
// The caller owns iteration/Err/Close, scope admission and finite stream budgets.
func (c *Client) DatabaseFiles(ctx context.Context) (*sql.Rows, error) {
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DatabaseFilesRows(ctx, c.native(), major)
}

// DatabaseFilesJSON independently consumes the same source into complete bounded
// JSON, retaining exact numeric bytes and actual container IDs where supported.
func (c *Client) DatabaseFilesJSON(ctx context.Context, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return storage.DatabaseFilesJSON(ctx, c.native(), major, resolved)
}

// EncryptionCounts returns the actual native count cursor for one independent
// encryption source. Connected architecture selects CDB_ only at the CDB root.
// Zero covers that visible dictionary, not closed/inaccessible containers.
// The caller owns Next/Scan/Err/Close and its admitted OPEN-state privileges.
func (c *Client) EncryptionCounts(ctx context.Context, kind database.EncryptionKind) (*sql.Rows, error) {
	major, root, err := c.encryptionScope(ctx, kind)
	if err != nil {
		return nil, err
	}
	return database.EncryptionRows(ctx, c.native(), major, root, kind)
}

// EncryptionCountsJSON owns a separate cursor for the same selected count.
// Complete shared rowset consumption includes provenance in the byte budget;
// any query/terminal/close/cancellation/domain failure returns no JSON.
func (c *Client) EncryptionCountsJSON(ctx context.Context, kind database.EncryptionKind, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	major, root, err := c.encryptionScope(ctx, kind)
	if err != nil {
		return nil, err
	}
	return database.EncryptionJSON(ctx, c.native(), major, root, kind, resolved)
}
func (c *Client) encryptionScope(ctx context.Context, kind database.EncryptionKind) (int, bool, error) {
	if _, err := database.EncryptionStatement(11, false, kind); err != nil {
		return 0, false, err
	}
	architecture, err := c.Architecture(ctx)
	if err != nil {
		return 0, false, err
	}
	if architecture.State != identity.Available {
		return 0, false, ErrArchitectureUnavailable
	}
	major, err := identity.Major(architecture.Version.Value)
	if err != nil {
		return 0, false, err
	}
	return major, architecture.Value == identity.CDB, nil
}
