package oracleinfo

import (
	"context"
	"errors"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

type nativeIdentityReader struct{ queryer Queryer }

// This is a bounded scalar-result adapter, not a transport/lease implementation.
// It borrows real rows and delegates their complete lifecycle to rowset.Read.
func (r nativeIdentityReader) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	rows, err := rowset.Query(ctx, r.queryer, statement.Text, statement.Args...)
	if err != nil {
		return session.Result{}, err
	}
	names, err := rows.Columns()
	if err != nil {
		return session.Result{}, &rowsetAdapterError{cause: errors.Join(err, rows.Err(), rows.Close(), rows.Err(), ctx.Err())}
	}
	schema := make([]rowset.Column, len(names))
	for i, name := range names {
		schema[i] = rowset.Column{Name: name, Kind: rowset.Text}
	}
	table, err := rowset.Read(ctx, rows, schema, rowset.Limits{MaxRows: 16, MaxBytes: 64 << 10})
	if err != nil {
		return session.Result{}, err
	}
	result := session.Result{Columns: make([]string, len(table.Columns)), Rows: make([][]session.Value, len(table.Rows))}
	for i, column := range table.Columns {
		result.Columns[i] = column.Name
	}
	for i, row := range table.Rows {
		result.Rows[i] = make([]session.Value, len(row))
		for j, value := range row {
			exact, err := session.ExactValue(value)
			if err != nil {
				return session.Result{}, &rowsetAdapterError{cause: err}
			}
			result.Rows[i][j] = exact
		}
	}
	return result, nil
}

type rowsetAdapterError struct{ cause error }

func (e *rowsetAdapterError) Error() string { return "identity native row adaptation failed" }
func (e *rowsetAdapterError) Unwrap() error { return e.cause }

func propertyReference(command string, number uint32) (reference.Reference, error) {
	return reference.NewReference("go-oracle-info", "identity", command, number)
}
func versionReferences() (identity.VersionReferences, error) {
	first, err := propertyReference("version", 1)
	if err != nil {
		return identity.VersionReferences{}, err
	}
	second, err := propertyReference("version", 2)
	return identity.VersionReferences{Legacy: first, Full: second}, err
}
func architectureReferences() (identity.ArchitectureReferences, error) {
	first, err := propertyReference("architecture", 1)
	if err != nil {
		return identity.ArchitectureReferences{}, err
	}
	second, err := propertyReference("architecture", 2)
	return identity.ArchitectureReferences{Version: first, Container: second}, err
}
func editionReferences() (identity.EditionReferences, error) {
	first, err := propertyReference("edition", 1)
	if err != nil {
		return identity.EditionReferences{}, err
	}
	second, err := propertyReference("edition", 2)
	if err != nil {
		return identity.EditionReferences{}, err
	}
	third, err := propertyReference("edition", 3)
	return identity.EditionReferences{Version: first, Code: second, Banners: third}, err
}
