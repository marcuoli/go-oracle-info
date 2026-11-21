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

// SavedStatesOptions admits the observed release and a root connection.
type SavedStatesOptions struct {
	OracleVersion string
	Scope         Scope
}

// SavedStatesRequest attaches a caller-owned statement reference.
type SavedStatesRequest struct {
	Options   SavedStatesOptions
	Reference reference.Reference
}

// SavedState is one dictionary definition. InstanceName names the configured
// member even when that instance is currently absent from GV$INSTANCE.
type SavedState struct {
	ContainerID   session.Value
	ContainerName session.Value
	InstanceName  session.Value
	ContainerUID  session.Value
	GUID          session.Value
	State         session.Value
	Restricted    session.Value
}

// SavedStatesResult contains independently observed saved definitions.
type SavedStatesResult struct {
	Provenance Provenance
	Rows       []SavedState
}

// SavedStatesStatement selects saved startup definitions in the admitted CDB root.
func SavedStatesStatement(options SavedStatesOptions) (string, error) {
	p, err := savedStatesPlan(options)
	return p.sql, err
}

func savedStatesPlan(options SavedStatesOptions) (selection, error) {
	p, err := profile(options.OracleVersion, options.Scope)
	if err != nil {
		return selection{}, err
	}
	if p < release1212 {
		return selection{}, ErrUnsupportedVersion
	}
	if options.Scope != Root {
		return selection{}, ErrUnsupportedScope
	}
	fields := []field{
		raw("CON_ID", rowset.Integer), raw("CON_NAME", rowset.Text), raw("INSTANCE_NAME", rowset.Text),
		exact("CON_UID"), projected("GUID", "RAWTOHEX(v.GUID)", rowset.Text),
		raw("STATE", rowset.Text), raw("RESTRICTED", rowset.Text),
	}
	provenance := Provenance{Source: "DBA_PDB_SAVED_STATES", OracleVersion: options.OracleVersion, Scope: options.Scope}
	return selectFields(provenance, fields, nil, "v.CON_ID >= 2 AND "+connectedContainer+" = 1", "v.CON_ID, v.INSTANCE_NAME"), nil
}

// DecodeSavedStates validates all rows without assigning a live reporting ID.
func DecodeSavedStates(result session.Result, options SavedStatesOptions) (SavedStatesResult, error) {
	p, err := savedStatesPlan(options)
	if err != nil {
		return SavedStatesResult{}, err
	}
	return decodeSavedStates(result, p)
}

func decodeSavedStates(result session.Result, p selection) (SavedStatesResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return SavedStatesResult{}, err
	}
	rows := make([]SavedState, len(result.Rows))
	for i, row := range result.Rows {
		v := SavedState{
			ContainerID: row[index["CON_ID"]], ContainerName: row[index["CON_NAME"]], InstanceName: row[index["INSTANCE_NAME"]],
			ContainerUID: row[index["CON_UID"]], GUID: row[index["GUID"]], State: row[index["STATE"]], Restricted: row[index["RESTRICTED"]],
		}
		if id(v.ContainerID) < 2 || !requiredText(v.ContainerName) || !requiredText(v.InstanceName) ||
			v.ContainerUID.Null || !unsigned(v.ContainerUID) || !guid(v.GUID) {
			return SavedStatesResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return SavedStatesResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSavedStates returns one complete buffered dictionary observation.
func ReadSavedStates(ctx context.Context, reader Reader, request SavedStatesRequest) (SavedStatesResult, error) {
	p, err := savedStatesPlan(request.Options)
	if err != nil {
		return SavedStatesResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return SavedStatesResult{}, err
	}
	facts, err := decodeSavedStates(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return SavedStatesResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SavedStatesRows returns the actual cursor and transfers its lifetime to the caller.
func SavedStatesRows(ctx context.Context, q rowset.Queryer, options SavedStatesOptions) (*sql.Rows, error) {
	p, err := savedStatesPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// SavedStatesJSON independently returns one complete bounded dictionary dataset.
func SavedStatesJSON(ctx context.Context, q rowset.Queryer, options SavedStatesOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := savedStatesPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeSavedStates(result, p)
		return err
	})
}
