package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// RegistryComponentsRequest supplies caller admission, a SQL reference, and buffered bounds.
type RegistryComponentsRequest struct {
	Options   RootDictionaryOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// RegistryComponent preserves the dictionary's raw registered component values.
// VersionFull is absent when the release does not expose that column.
type RegistryComponent struct {
	ContainerID session.Value
	ID          session.Value
	Name        session.Value
	Version     session.Value
	Status      session.Value
	Modified    session.Value
	VersionFull *session.Value
}

// RegistryComponentsResult retains the visible rows and their source provenance.
type RegistryComponentsResult struct {
	Provenance Provenance
	Rows       []RegistryComponent
}

// RegistryComponentsStatement selects the reviewed source without SQL truncation.
func RegistryComponentsStatement(options RootDictionaryOptions) (string, error) {
	p, err := registryComponentsPlan(options)
	return p.sql, err
}

func registryComponentsPlan(options RootDictionaryOptions) (selection, error) {
	_, err := rootDictionaryProfile(options)
	if err != nil {
		return selection{}, err
	}
	p := Provenance{Source: "CDB_REGISTRY", OracleVersion: options.OracleVersion, Scope: options.Scope}
	fields := []field{
		raw("CON_ID", rowset.Integer),
		raw("COMP_ID", rowset.Text),
		raw("COMP_NAME", rowset.Text),
		raw("VERSION", rowset.Text),
	}
	if major, _ := oracleversion.LegacyFactMajor(options.OracleVersion); major >= 18 {
		fields = append(fields, raw("VERSION_FULL", rowset.Text))
		p.OptionalColumns = []string{"VERSION_FULL"}
	}
	fields = append(fields, raw("STATUS", rowset.Text), raw("MODIFIED", rowset.Text))
	return selectFields(p, fields, nil, rootDictionaryPredicate, "v.CON_ID, v.COMP_ID"), nil
}

// DecodeRegistryComponents validates the supplied complete shape before returning any rows.
func DecodeRegistryComponents(result session.Result, options RootDictionaryOptions) (RegistryComponentsResult, error) {
	p, err := registryComponentsPlan(options)
	if err != nil {
		return RegistryComponentsResult{}, err
	}
	return decodeRegistryComponents(result, p)
}

func decodeRegistryComponents(result session.Result, p selection) (RegistryComponentsResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return RegistryComponentsResult{}, err
	}
	rows := make([]RegistryComponent, len(result.Rows))
	for i, row := range result.Rows {
		item := RegistryComponent{
			ContainerID: row[index["CON_ID"]],
			ID:          row[index["COMP_ID"]],
			Name:        row[index["COMP_NAME"]],
			Version:     row[index["VERSION"]],
			Status:      row[index["STATUS"]],
			Modified:    row[index["MODIFIED"]],
			VersionFull: optional(row, index, "VERSION_FULL"),
		}
		if item.ContainerID.Null || !unsigned(item.ContainerID) {
			return RegistryComponentsResult{}, ErrInvalidEvidence
		}
		rows[i] = item
	}
	return RegistryComponentsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadRegistryComponents executes once over the caller's admitted bounded reader.
func ReadRegistryComponents(ctx context.Context, reader Reader, request RegistryComponentsRequest) (RegistryComponentsResult, error) {
	p, err := registryComponentsPlan(request.Options)
	if err != nil {
		return RegistryComponentsResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return RegistryComponentsResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return RegistryComponentsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return RegistryComponentsResult{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
	}
	facts, decodeErr := decodeRegistryComponents(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return RegistryComponentsResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// RegistryComponentsRows transfers the actual native cursor to its owner.
func RegistryComponentsRows(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions) (*sql.Rows, error) {
	p, err := registryComponentsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, sourceFailure(p.provenance.Source, err, true)
	}
	return rows, nil
}

// RegistryComponentsJSON consumes a separate complete bounded cursor.
func RegistryComponentsJSON(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := registryComponentsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeRegistryComponents(r, p)
		return err
	})
}
