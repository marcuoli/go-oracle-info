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

// Location preserves the observed session container and instance identifiers.
type Location struct {
	Name        session.Value
	ContainerID session.Value
	InstanceID  session.Value
}

func sessionLocationPlan(version string) (selection, error) {
	if _, err := profile(version, CurrentContainer); err != nil {
		return selection{}, err
	}
	return selection{
		sql: "SELECT SYS_CONTEXT('USERENV','CON_NAME') AS CONTAINER, SYS_CONTEXT('USERENV','CON_ID') AS CON_ID, SYS_CONTEXT('USERENV','INSTANCE') AS INSTANCE_ID FROM DUAL",
		schema: []rowset.Column{
			{Name: "CONTAINER", Kind: rowset.Text},
			{Name: "CON_ID", Kind: rowset.Text},
			{Name: "INSTANCE_ID", Kind: rowset.Text},
		},
		provenance: Provenance{Source: "SYS_CONTEXT", OracleVersion: version, Scope: CurrentContainer},
	}, nil
}

// SessionLocationStatement selects the reviewed session container and instance identifiers projection.
func SessionLocationStatement(version string) (string, error) {
	p, err := sessionLocationPlan(version)
	return p.sql, err
}

// DecodeSessionLocation validates the shape without making a transport-completeness claim.
func DecodeSessionLocation(result session.Result, version string) ([]Location, error) {
	p, err := sessionLocationPlan(version)
	if err != nil {
		return nil, err
	}
	return decodeSessionLocation(result, p)
}

func decodeSessionLocation(result session.Result, p selection) ([]Location, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return nil, err
	}
	if len(result.Rows) != 1 {
		return nil, ErrInvalidEvidence
	}
	rows := make([]Location, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Location{
			Name:        row[index["CONTAINER"]],
			ContainerID: row[index["CON_ID"]],
			InstanceID:  row[index["INSTANCE_ID"]],
		}
	}
	return rows, nil
}

// ReadSessionLocation returns a complete buffered observation from the admitted reader.
func ReadSessionLocation(ctx context.Context, reader Reader, version string, ref reference.Reference) ([]Location, error) {
	p, err := sessionLocationPlan(version)
	if err != nil {
		return nil, err
	}
	result, err := readSource(ctx, reader, p, ref)
	if err != nil {
		return nil, err
	}
	rows, err := decodeSessionLocation(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, sourceFailure(p.provenance.Source, err, false)
	}
	return rows, nil
}

// SessionLocationRows returns a native cursor owned by the caller.
func SessionLocationRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := sessionLocationPlan(version)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// SessionLocationJSON reads and validates the complete cursor within the supplied bounds.
func SessionLocationJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sessionLocationPlan(version)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeSessionLocation(result, p)
		return err
	})
}
