package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/marcuoli/go-oracle-info/dblinks"
	"github.com/marcuoli/go-oracle-info/directories"
	"github.com/marcuoli/go-oracle-info/grants"
	"github.com/marcuoli/go-oracle-info/metadata"
	"github.com/marcuoli/go-oracle-info/networkacl"
	"github.com/marcuoli/go-oracle-info/sequences"
	"github.com/marcuoli/go-oracle-info/sqlplan"
	"github.com/marcuoli/go-oracle-info/triggers"
	"github.com/marcuoli/go-oracle-info/users"
)

// DictionaryFilter selects exact dictionary owners and names. Empty slices keep
// every visible row; the caller owns schema/role/user policy and authorization.
// Grants interpret Names as grantees/roles; baselines interpret Owners as parsing
// schemas and Names as plan names. Users/profiles accept only Names.
type DictionaryFilter struct {
	Owners []string
	Names  []string
}

// Users returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) Users(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := users.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.Rows(ctx, c.native(), options)
}

// UsersJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) UsersJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = users.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.JSON(ctx, c.native(), options, resolved)
}

// UserAccountCount returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) UserAccountCount(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := users.AccountCountStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.AccountCountRows(ctx, c.native(), options)
}

// UserAccountCountJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) UserAccountCountJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = users.AccountCountStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.AccountCountJSON(ctx, c.native(), options, resolved)
}

// TableMetadata returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) TableMetadata(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := users.TableMetadataStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.TableMetadataRows(ctx, c.native(), options)
}

// TableMetadataJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) TableMetadataJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := users.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = users.TableMetadataStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return users.TableMetadataJSON(ctx, c.native(), options, resolved)
}

// Sequences returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) Sequences(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := sequences.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := sequences.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sequences.Rows(ctx, c.native(), options)
}

// SequencesJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) SequencesJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := sequences.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = sequences.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sequences.JSON(ctx, c.native(), options, resolved)
}

// Directories returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) Directories(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := directories.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := directories.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return directories.Rows(ctx, c.native(), options)
}

// DirectoriesJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) DirectoriesJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := directories.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = directories.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return directories.JSON(ctx, c.native(), options, resolved)
}

// DatabaseLinks returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) DatabaseLinks(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := dblinks.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := dblinks.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return dblinks.Rows(ctx, c.native(), options)
}

// DatabaseLinksJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) DatabaseLinksJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := dblinks.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = dblinks.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return dblinks.JSON(ctx, c.native(), options, resolved)
}

// Triggers returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) Triggers(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := triggers.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := triggers.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return triggers.Rows(ctx, c.native(), options)
}

// TriggersJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) TriggersJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := triggers.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = triggers.Statement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return triggers.JSON(ctx, c.native(), options, resolved)
}

// DirectObjectGrants returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) DirectObjectGrants(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := grants.DirectStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.DirectRows(ctx, c.native(), options)
}

// DirectObjectGrantsJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) DirectObjectGrantsJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = grants.DirectStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.DirectJSON(ctx, c.native(), options, resolved)
}

// DirectColumnGrants returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) DirectColumnGrants(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := grants.DirectColumnStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.DirectColumnRows(ctx, c.native(), options)
}

// DirectColumnGrantsJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) DirectColumnGrantsJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = grants.DirectColumnStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.DirectColumnJSON(ctx, c.native(), options, resolved)
}

// RoleObjectGrants returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) RoleObjectGrants(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := grants.RoleStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.RoleRows(ctx, c.native(), options)
}

// RoleObjectGrantsJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) RoleObjectGrantsJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := grants.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = grants.RoleStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return grants.RoleJSON(ctx, c.native(), options, resolved)
}

// SQLProfiles returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) SQLProfiles(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := sqlplan.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := sqlplan.ProfileStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sqlplan.ProfileRows(ctx, c.native(), options)
}

// SQLProfilesJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) SQLProfilesJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := sqlplan.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = sqlplan.ProfileStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sqlplan.ProfileJSON(ctx, c.native(), options, resolved)
}

// SQLPlanBaselines returns the actual caller-native cursor after observed-version
// selection. The caller owns iteration, Err/Close, budgets and session affinity.
func (c *Client) SQLPlanBaselines(ctx context.Context, filter DictionaryFilter) (*sql.Rows, error) {
	options := sqlplan.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err := sqlplan.BaselineStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sqlplan.BaselineRows(ctx, c.native(), options)
}

// SQLPlanBaselinesJSON independently consumes the same source as bounded JSON.
// It publishes no result after query/terminal/close/cancellation/budget failure.
func (c *Client) SQLPlanBaselinesJSON(ctx context.Context, filter DictionaryFilter, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options := sqlplan.Options{OracleMajor: 11, Owners: filter.Owners, Names: filter.Names}
	if _, _, err = sqlplan.BaselineStatement(options); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	options.OracleMajor = major
	return sqlplan.BaselineJSON(ctx, c.native(), options, resolved)
}

// NetworkACL returns the selected DBA/user host/wallet source's actual native
// cursor. No DBA permission failure falls back to current-user privileges.
func (c *Client) NetworkACL(ctx context.Context, dataset networkacl.Dataset) (*sql.Rows, error) {
	if _, _, err := networkacl.Statement(networkacl.Options{OracleMajor: 19, Dataset: dataset}); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return networkacl.Rows(ctx, c.native(), networkacl.Options{OracleMajor: major, Dataset: dataset})
}

// NetworkACLJSON independently consumes one selected ACL visibility dataset.
func (c *Client) NetworkACLJSON(ctx context.Context, dataset networkacl.Dataset, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	if _, _, err = networkacl.Statement(networkacl.Options{OracleMajor: 19, Dataset: dataset}); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	return networkacl.JSON(ctx, c.native(), networkacl.Options{OracleMajor: major, Dataset: dataset}, resolved)
}

// MetadataOptions selects one supported object and a complete UTF-8 DDL cap.
// Owner is required for a trigger. Zero MaxDDLBytes uses 8MiB.
type MetadataOptions struct {
	Kind        metadata.Kind
	Name        string
	Owner       string
	MaxDDLBytes int
}

// MetadataDDL returns the native CLOB cursor for explicitly selected metadata.
// The caller owns complete CLOB consumption, length/budget checks and Err/Close.
// Captured DDL is evidence only; inherited session transforms remain unchanged.
func (c *Client) MetadataDDL(ctx context.Context, options MetadataOptions) (*sql.Rows, error) {
	selected := metadata.Options{OracleMajor: 11, Kind: options.Kind, Name: options.Name, Owner: options.Owner, MaxDDLBytes: options.MaxDDLBytes}
	if _, _, err := metadata.Statement(selected); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	selected.OracleMajor = major
	return metadata.Rows(ctx, c.native(), selected)
}

// MetadataDDLJSON independently captures a complete, length-checked CLOB as JSON.
// It changes no session transforms and returns no prefix on any failure.
func (c *Client) MetadataDDLJSON(ctx context.Context, options MetadataOptions, limits ...JSONLimits) (json.RawMessage, error) {
	resolved, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	selected := metadata.Options{OracleMajor: 11, Kind: options.Kind, Name: options.Name, Owner: options.Owner, MaxDDLBytes: options.MaxDDLBytes}
	if _, _, err = metadata.Statement(selected); err != nil {
		return nil, err
	}
	major, err := c.major(ctx)
	if err != nil {
		return nil, err
	}
	selected.OracleMajor = major
	return metadata.JSON(ctx, c.native(), selected, resolved)
}
