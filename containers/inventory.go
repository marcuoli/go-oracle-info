package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const connectedContainer = "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))"

// InventoryOptions admits the observed release and either root-visible or
// current-container scope. The handle's owner establishes its actual scope.
type InventoryOptions struct {
	OracleVersion string
	Scope         Scope
}

// InventoryRequest attaches a caller-owned statement reference.
type InventoryRequest struct {
	Options   InventoryOptions
	Reference reference.Reference
}

// Container preserves one observed member/container row. Byte counts and DBID
// are exact text. GUID is hexadecimal. Nil pointers denote unprojected columns.
type Container struct {
	InstanceID                 session.Value
	ContainerID                session.Value
	SourceContainerID          session.Value
	Name                       session.Value
	OpenMode                   session.Value
	Restricted                 session.Value
	DBID                       session.Value
	ContainerUID               session.Value
	GUID                       session.Value
	TotalSizeBytes             *session.Value
	MaxSizeBytes               *session.Value
	ApplicationRoot            *session.Value
	ApplicationPDB             *session.Value
	ApplicationSeed            *session.Value
	ApplicationRootContainerID *session.Value
}

// InventoryResult contains the complete ordered rows when a read succeeds.
// An empty read returns nonnil empty Rows.
type InventoryResult struct {
	Provenance Provenance
	Rows       []Container
}

// InventoryStatement selects qualified visible rows from GV$CONTAINERS.
func InventoryStatement(options InventoryOptions) (string, error) {
	p, err := inventoryPlan(options)
	return p.sql, err
}

func inventoryPlan(options InventoryOptions) (selection, error) {
	p, err := profile(options.OracleVersion, options.Scope)
	if err != nil {
		return selection{}, err
	}
	fields := []field{
		raw("INST_ID", rowset.Integer), raw("CON_ID", rowset.Integer),
		projected("SOURCE_CON_ID", connectedContainer, rowset.Integer),
		raw("NAME", rowset.Text), raw("OPEN_MODE", rowset.Text), raw("RESTRICTED", rowset.Text),
		exact("DBID"), exact("CON_UID"), projected("GUID", "RAWTOHEX(v.GUID)", rowset.Text),
	}
	var extras []field
	if p >= release1212 {
		extras = append(extras, exact("TOTAL_SIZE"))
	}
	if p >= release122 {
		extras = append(extras, exact("MAX_SIZE"), raw("APPLICATION_ROOT", rowset.Text),
			raw("APPLICATION_PDB", rowset.Text), raw("APPLICATION_SEED", rowset.Text),
			raw("APPLICATION_ROOT_CON_ID", rowset.Integer))
	}
	provenance := Provenance{Source: "GV$CONTAINERS", OracleVersion: options.OracleVersion, Scope: options.Scope}
	return selectFields(provenance.Source, provenance, fields, extras, memberPredicate(options.Scope), "v.CON_ID, v.INST_ID"), nil
}

// DecodeInventory validates all supplied rows and preserves their raw evidence.
func DecodeInventory(result session.Result, options InventoryOptions) (InventoryResult, error) {
	p, err := inventoryPlan(options)
	if err != nil {
		return InventoryResult{}, err
	}
	return decodeInventory(result, p)
}

func decodeInventory(result session.Result, p selection) (InventoryResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return InventoryResult{}, err
	}
	rows := make([]Container, len(result.Rows))
	for i, row := range result.Rows {
		v := Container{
			InstanceID: row[index["INST_ID"]], ContainerID: row[index["CON_ID"]], SourceContainerID: row[index["SOURCE_CON_ID"]],
			Name: row[index["NAME"]], OpenMode: row[index["OPEN_MODE"]], Restricted: row[index["RESTRICTED"]],
			DBID: row[index["DBID"]], ContainerUID: row[index["CON_UID"]], GUID: row[index["GUID"]],
			TotalSizeBytes: optional(row, index, "TOTAL_SIZE"), MaxSizeBytes: optional(row, index, "MAX_SIZE"),
			ApplicationRoot: optional(row, index, "APPLICATION_ROOT"), ApplicationPDB: optional(row, index, "APPLICATION_PDB"),
			ApplicationSeed: optional(row, index, "APPLICATION_SEED"), ApplicationRootContainerID: optional(row, index, "APPLICATION_ROOT_CON_ID"),
		}
		if !validMember(v.InstanceID, v.ContainerID, v.SourceContainerID, p.provenance.Scope) || !requiredText(v.Name) ||
			!unsigned(v.DBID) || !unsigned(v.ContainerUID) || !guid(v.GUID) ||
			(v.TotalSizeBytes != nil && !unsigned(*v.TotalSizeBytes)) || (v.MaxSizeBytes != nil && !unsigned(*v.MaxSizeBytes)) ||
			(v.ApplicationRootContainerID != nil && !v.ApplicationRootContainerID.Null && id(*v.ApplicationRootContainerID) < 0) {
			return InventoryResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return InventoryResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadInventory reads one complete buffered inventory from an admitted reader.
func ReadInventory(ctx context.Context, reader Reader, request InventoryRequest) (InventoryResult, error) {
	p, err := inventoryPlan(request.Options)
	if err != nil {
		return InventoryResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return InventoryResult{}, err
	}
	facts, err := decodeInventory(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return InventoryResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// InventoryRows returns the actual cursor. The caller owns Next, Scan, Err, and Close.
func InventoryRows(ctx context.Context, q rowset.Queryer, options InventoryOptions) (*sql.Rows, error) {
	p, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// InventoryJSON returns one complete native rowset and provenance within the limits.
func InventoryJSON(ctx context.Context, q rowset.Queryer, options InventoryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeInventory(result, p)
		return err
	})
}
