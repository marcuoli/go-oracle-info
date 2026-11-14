// This connection-free example uses complete fixture results. A real application
// supplies an already-admitted reader; the identity package opens no connection.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

type fixtureReader struct{}

func (fixtureReader) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	if err := ctx.Err(); err != nil {
		return session.Result{}, err
	}
	var columns, values []string
	switch statement.Reference.Number() {
	case 1, 3, 5:
		columns, values = []string{"VERSION"}, []string{"23.0.0.0.0"}
	case 2:
		columns, values = []string{"VERSION"}, []string{"23.26.1.0.0"}
	case 4:
		columns, values = []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}, []string{"YES", "3", "EXAMPLE_PDB"}
	case 6:
		columns, values = []string{"EDITION"}, []string{"EE"}
	default:
		return session.Result{}, errors.New("unrecognized fixture query reference")
	}
	row := make([]session.Value, len(values))
	for i, value := range values {
		row[i] = session.Value{Text: value}
	}
	return session.Result{Columns: columns, Rows: [][]session.Value{row}}, nil
}

func run(ctx context.Context, reader identity.Reader) error {
	refs := make([]reference.Reference, 7)
	for i := range refs {
		ref, err := reference.NewReference("example", "identity", "properties", uint32(i+1))
		if err != nil {
			return err
		}
		refs[i] = ref
	}
	version, err := identity.ReadVersion(ctx, reader, identity.VersionReferences{Legacy: refs[0], Full: refs[1]})
	// Errors can coexist with a valid retained legacy value. Check the error
	// first, then the exact precision needed by this particular example.
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}
	if version.State != identity.Available || version.Precision != identity.FullPrecision || version.Fallback {
		return errors.New("example requires an observed full numeric version")
	}
	architecture, err := identity.ReadArchitecture(ctx, reader, identity.ArchitectureReferences{Version: refs[2], Container: refs[3]})
	if err != nil {
		return fmt.Errorf("read architecture: %w", err)
	}
	if architecture.State != identity.Available || architecture.Method != identity.DictionaryArchitecture || architecture.Fallback {
		return errors.New("example requires observed connected-container signals")
	}
	edition, err := identity.ReadEdition(ctx, reader, identity.EditionReferences{Version: refs[4], Code: refs[5], Banners: refs[6]})
	if err != nil {
		return fmt.Errorf("read edition: %w", err)
	}
	if edition.State != identity.Available || edition.Precision != identity.ObservedEditionCode || edition.Fallback {
		return errors.New("example requires an observed edition code")
	}
	fmt.Printf("fixture: %s / %s (%s) / %s\n", architecture.Value, version.Value, version.Precision, edition.Value)
	return nil
}

func main() {
	if err := run(context.Background(), fixtureReader{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
