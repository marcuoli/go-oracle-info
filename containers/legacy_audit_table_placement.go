package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// LegacyAuditTablePlacementRequest supplies caller admission, a SQL reference, and buffered bounds.
type LegacyAuditTablePlacementRequest struct {
	Options   RootDictionaryOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// LegacyAuditTable identifies one visible legacy audit table placement.
// TablespaceName preserves SQL NULL for a non-scalar placement.
type LegacyAuditTable struct {
	ContainerID, Owner, TableName, TablespaceName session.Value
}

// LegacyAuditTablePlacementResult retains the visible rows and their source provenance.
type LegacyAuditTablePlacementResult struct {
	Provenance Provenance
	Rows       []LegacyAuditTable
}

// LegacyAuditTablePlacementStatement selects the reviewed source without SQL truncation.
func LegacyAuditTablePlacementStatement(options RootDictionaryOptions) (string, error) {
	p, err := legacyAuditTablePlacementPlan(options)
	return p.sql, err
}

func legacyAuditTablePlacementPlan(options RootDictionaryOptions) (selection, error) {
	if _, err := rootDictionaryProfile(options); err != nil {
		return selection{}, err
	}
	p := Provenance{Source: "CDB_TABLES", OracleVersion: options.OracleVersion, Scope: options.Scope}
	fields := []field{
		raw("CON_ID", rowset.Integer),
		raw("OWNER", rowset.Text),
		raw("TABLE_NAME", rowset.Text),
		raw("TABLESPACE_NAME", rowset.Text),
	}
	const legacyKeys = "NLSSORT(v.OWNER, 'NLS_SORT=BINARY') = NLSSORT('SYS', 'NLS_SORT=BINARY')" +
		" AND NLSSORT(v.TABLE_NAME, 'NLS_SORT=BINARY') IN (NLSSORT('AUD$', 'NLS_SORT=BINARY'), NLSSORT('FGA_LOG$', 'NLS_SORT=BINARY'))"
	return selectFields(p, fields, nil, rootDictionaryPredicate+" AND "+legacyKeys, "v.CON_ID, v.OWNER, v.TABLE_NAME"), nil
}

// DecodeLegacyAuditTablePlacement validates the supplied complete shape before returning any rows.
func DecodeLegacyAuditTablePlacement(result session.Result, options RootDictionaryOptions) (LegacyAuditTablePlacementResult, error) {
	p, err := legacyAuditTablePlacementPlan(options)
	if err != nil {
		return LegacyAuditTablePlacementResult{}, err
	}
	return decodeLegacyAuditTablePlacement(result, p)
}

func decodeLegacyAuditTablePlacement(result session.Result, p selection) (LegacyAuditTablePlacementResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return LegacyAuditTablePlacementResult{}, err
	}
	rows := make([]LegacyAuditTable, len(result.Rows))
	for i, row := range result.Rows {
		item := LegacyAuditTable{
			ContainerID: row[index["CON_ID"]], Owner: row[index["OWNER"]],
			TableName: row[index["TABLE_NAME"]], TablespaceName: row[index["TABLESPACE_NAME"]],
		}
		if item.ContainerID.Null || !unsigned(item.ContainerID) || item.Owner.Text != "SYS" ||
			(item.TableName.Text != "AUD$" && item.TableName.Text != "FGA_LOG$") {
			return LegacyAuditTablePlacementResult{}, ErrInvalidEvidence
		}
		rows[i] = item
	}
	return LegacyAuditTablePlacementResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadLegacyAuditTablePlacement executes once over the caller's admitted bounded reader.
func ReadLegacyAuditTablePlacement(ctx context.Context, reader Reader, request LegacyAuditTablePlacementRequest) (LegacyAuditTablePlacementResult, error) {
	p, err := legacyAuditTablePlacementPlan(request.Options)
	if err != nil {
		return LegacyAuditTablePlacementResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return LegacyAuditTablePlacementResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return LegacyAuditTablePlacementResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return LegacyAuditTablePlacementResult{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
	}
	facts, decodeErr := decodeLegacyAuditTablePlacement(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return LegacyAuditTablePlacementResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// LegacyAuditTablePlacementRows transfers the actual native cursor to its owner.
func LegacyAuditTablePlacementRows(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions) (*sql.Rows, error) {
	p, err := legacyAuditTablePlacementPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, sourceFailure(p.provenance.Source, err, true)
	}
	return rows, nil
}

// LegacyAuditTablePlacementJSON consumes a separate complete bounded cursor.
func LegacyAuditTablePlacementJSON(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := legacyAuditTablePlacementPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeLegacyAuditTablePlacement(r, p)
		return err
	})
}
