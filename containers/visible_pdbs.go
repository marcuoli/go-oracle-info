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

// VisiblePDB preserves the observed PDB identity and open mode on the reporting instance.
type VisiblePDB struct {
	ContainerID session.Value
	Name        session.Value
	OpenMode    session.Value
	InstanceID  session.Value
}

func visiblePDBPlan(version string) (selection, error) {
	if _, err := profile(version, Root); err != nil {
		return selection{}, err
	}
	return selection{
		sql: "SELECT p.CON_ID AS CON_ID, p.NAME AS NAME, p.OPEN_MODE AS OPEN_MODE, SYS_CONTEXT('USERENV','INSTANCE') AS INSTANCE_ID FROM V$PDBS p WHERE p.CON_ID > 2 AND SYS_CONTEXT('USERENV','CON_ID') = '1' ORDER BY p.NAME",
		schema: []rowset.Column{
			{Name: "CON_ID", Kind: rowset.Integer},
			{Name: "NAME", Kind: rowset.Text},
			{Name: "OPEN_MODE", Kind: rowset.Text},
			{Name: "INSTANCE_ID", Kind: rowset.Text},
		},
		provenance: Provenance{Source: "V$PDBS", OracleVersion: version, Scope: Root},
	}, nil
}

// VisiblePDBsStatement selects the reviewed PDB identity and open mode on the reporting instance projection.
func VisiblePDBsStatement(version string) (string, error) {
	p, err := visiblePDBPlan(version)
	return p.sql, err
}

// DecodeVisiblePDBs validates the shape without making a transport-completeness claim.
func DecodeVisiblePDBs(result session.Result, version string) ([]VisiblePDB, error) {
	p, err := visiblePDBPlan(version)
	if err != nil {
		return nil, err
	}
	return decodeVisiblePDBs(result, p)
}

func decodeVisiblePDBs(result session.Result, p selection) ([]VisiblePDB, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return nil, err
	}
	rows := make([]VisiblePDB, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = VisiblePDB{
			ContainerID: row[index["CON_ID"]],
			Name:        row[index["NAME"]],
			OpenMode:    row[index["OPEN_MODE"]],
			InstanceID:  row[index["INSTANCE_ID"]],
		}
	}
	return rows, nil
}

// ReadVisiblePDBs returns a complete buffered observation from the admitted reader.
func ReadVisiblePDBs(ctx context.Context, reader Reader, version string, ref reference.Reference) ([]VisiblePDB, error) {
	p, err := visiblePDBPlan(version)
	if err != nil {
		return nil, err
	}
	result, err := readSource(ctx, reader, p, ref)
	if err != nil {
		return nil, err
	}
	rows, err := decodeVisiblePDBs(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, sourceFailure(p.provenance.Source, err, false)
	}
	return rows, nil
}

// VisiblePDBsRows returns a native cursor owned by the caller.
func VisiblePDBsRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := visiblePDBPlan(version)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// VisiblePDBsJSON reads and validates the complete cursor within the supplied bounds.
func VisiblePDBsJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := visiblePDBPlan(version)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeVisiblePDBs(result, p)
		return err
	})
}
