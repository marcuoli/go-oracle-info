package oracleinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/parameters"
)

// PDBParameters returns the caller-owned cursor for visible PDB-attributed
// instance settings. An empty pdbIDs selects all visible PDB-attributed rows from
// the root (CON_ID > 2), or only the connected PDB on a direct PDB handle.
func (c *Client) PDBParameters(ctx context.Context, pdbIDs []int64, filter ParameterFilter) (*sql.Rows, error) {
	options, err := c.pdbParameterOptions(ctx, pdbIDs, filter)
	if err != nil {
		return nil, err
	}
	return parameters.ContainerSystemRows(ctx, c.native(), options)
}

// PDBParametersJSON consumes a separate complete cursor under the given limits.
func (c *Client) PDBParametersJSON(ctx context.Context, pdbIDs []int64, filter ParameterFilter, limits ...JSONLimits) (json.RawMessage, error) {
	bounds, err := jsonLimits(limits)
	if err != nil {
		return nil, err
	}
	options, err := c.pdbParameterOptions(ctx, pdbIDs, filter)
	if err != nil {
		return nil, err
	}
	return parameters.ContainerSystemJSON(ctx, c.native(), options, bounds)
}

func (c *Client) pdbParameterOptions(ctx context.Context, pdbIDs []int64, filter ParameterFilter) (parameters.ContainerSystemOptions, error) {
	options := parameters.ContainerSystemOptions{
		OracleVersion: "12.1.0.1.0", CurrentContainerID: 1,
		Source: parameters.ContainerSystemPDBGlobal, Filter: filter, ContainerIDs: pdbIDs,
	}
	if _, _, err := parameters.ContainerSystemStatement(options); err != nil {
		return parameters.ContainerSystemOptions{}, err
	}
	architecture, err := c.Architecture(ctx)
	if err != nil {
		return parameters.ContainerSystemOptions{}, err
	}
	if architecture.Version.State != identity.Available {
		return parameters.ContainerSystemOptions{}, ErrVersionUnavailable
	}
	if architecture.State != identity.Available {
		return parameters.ContainerSystemOptions{}, ErrArchitectureUnavailable
	}
	options.OracleVersion = architecture.Version.Value
	options.CurrentContainerID = 0
	if architecture.Value != identity.NonCDB {
		id, err := strconv.ParseInt(architecture.ContainerID.Value, 10, 64)
		if err != nil {
			return parameters.ContainerSystemOptions{}, ErrArchitectureUnavailable
		}
		options.CurrentContainerID = id
	}
	return options, nil
}
