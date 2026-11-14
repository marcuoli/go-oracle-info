package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func architectureResult(cdb, id, name identity.Text) session.Result {
	return session.Result{Columns: []string{"container_name", "CDB", "CONTAINER_ID"}, Rows: [][]session.Value{{
		{Text: name.Value, Null: name.State == identity.Null},
		{Text: cdb.Value, Null: cdb.State == identity.Null},
		{Text: id.Value, Null: id.State == identity.Null},
	}}}
}
func TestReadArchitectureUsesConnectedSignals(t *testing.T) {
	t.Parallel()
	const wantSQL = "SELECT CDB AS CDB, SYS_CONTEXT('USERENV','CON_ID') AS CONTAINER_ID, SYS_CONTEXT('USERENV','CON_NAME') AS CONTAINER_NAME FROM V$DATABASE"
	if identity.ConnectedArchitectureSQL != wantSQL {
		t.Fatal("atomic current-container projection changed")
	}
	for _, tc := range []struct {
		name, cdb, id, container string
		want                     identity.Architecture
		invalid                  bool
	}{
		{"root", "YES", "1", "CDB$ROOT", identity.CDB, false},
		{"seed", "YES", "2", "PDB$SEED", identity.PDB, false},
		{"pdb", "YES", "7", "ApplicationPDB", identity.PDB, false},
		{"non-cdb", "NO", "0", "LEGACY", identity.NonCDB, false},
		{"root-wrong-name", "YES", "1", "ApplicationPDB", "", true},
		{"root-wrong-id", "YES", "7", "CDB$ROOT", "", true},
		{"seed-wrong-name", "YES", "2", "ApplicationPDB", "", true},
		{"seed-wrong-id", "YES", "7", "PDB$SEED", "", true},
		{"non-cdb-root", "NO", "0", "CDB$ROOT", "", true},
		{"non-cdb-positive-id", "NO", "7", "ApplicationPDB", "", true},
		{"cdb-zero", "YES", "0", "CDB$ROOT", "", true},
		{"unknown-parent-flag", "Y", "1", "CDB$ROOT", "", true},
		{"negative-id", "YES", "-1", "CDB$ROOT", "", true},
		{"decimal-id", "YES", "1.0", "CDB$ROOT", "", true},
		{"huge-id", "YES", "4294967296", "ApplicationPDB", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := architectureRefs(t)
			cdb, id, name := available(tc.cdb), available(tc.id), available(tc.container)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("19.0.0.0.0")), {ref: refs.Container, statement: wantSQL, result: architectureResult(cdb, id, name)}}}
			got, err := identity.ReadArchitecture(ctx, reader, refs)
			reader.done()
			if got.DatabaseCDB != cdb || got.ContainerID != id || got.ContainerName != name || got.Method != identity.DictionaryArchitecture || got.Fallback || got.Source != identity.ConnectedContainerSource {
				t.Fatalf("parent/current provenance changed: %#v", got)
			}
			if tc.invalid {
				if !errors.Is(err, identity.ErrInvalidEvidence) || got.State != identity.Unavailable || got.Value != "" {
					t.Fatalf("contradiction accepted: %#v %v", got, err)
				}
			} else if err != nil || got.State != identity.Available || got.Value != tc.want {
				t.Fatalf("wrong connected architecture: %#v %v", got, err)
			}
		})
	}
}
func TestReadArchitectureRetainsMissingSignalsWithoutGuessing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		cdb, id, container identity.Text
	}{
		{"parent-null", identity.Text{State: identity.Null}, available("7"), available("PDB")},
		{"id-null", available("YES"), identity.Text{State: identity.Null}, available("PDB")},
		{"name-null", available("YES"), available("7"), identity.Text{State: identity.Null}},
		{"id-empty", available("YES"), available(""), available("PDB")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := architectureRefs(t)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("19.0.0.0.0")), {ref: refs.Container, statement: identity.ConnectedArchitectureSQL, result: architectureResult(tc.cdb, tc.id, tc.container)}}}
			got, err := identity.ReadArchitecture(ctx, reader, refs)
			reader.done()
			if err != nil || got.State != identity.Unavailable || got.Value != "" || got.DatabaseCDB != tc.cdb || got.ContainerID != tc.id || got.ContainerName != tc.container {
				t.Fatalf("missing signals inferred: %#v %v", got, err)
			}
		})
	}
}
func TestReadArchitectureLegacyIsExplicitlyReleaseConstrained(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	refs := architectureRefs(t)
	reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("11.2.0.4.0"))}}
	got, err := identity.ReadArchitecture(ctx, reader, refs)
	reader.done()
	if err != nil || got.State != identity.Available || got.Value != identity.NonCDB || got.Method != identity.LegacyReleaseArchitecture || !got.Fallback || got.Source != identity.InstanceVersionSource {
		t.Fatalf("legacy source not explicit: %#v %v", got, err)
	}
	if got.DatabaseCDB.State != identity.Unavailable || got.ContainerID.State != identity.Unavailable || got.ContainerName.State != identity.Unavailable {
		t.Fatal("legacy dictionary signals fabricated")
	}
}
func TestReadArchitecturePreservesNoRowsAndStopsReadFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("unavailable controlfile/grant")
	for _, tc := range []struct {
		name   string
		result session.Result
		err    error
		want   identity.State
	}{
		{"no-rows", session.Result{Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}}, nil, identity.NoRows},
		{"failed", session.Result{}, failure, identity.Unavailable},
		{"bad-shape", session.Result{Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}, Rows: [][]session.Value{{{Text: "YES"}}}}, identity.ErrInvalidEvidence, identity.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := architectureRefs(t)
			queryErr := tc.err
			if errors.Is(queryErr, identity.ErrInvalidEvidence) {
				queryErr = nil
			}
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("19.0.0.0.0")), {ref: refs.Container, statement: identity.ConnectedArchitectureSQL, result: tc.result, err: queryErr}}}
			got, err := identity.ReadArchitecture(ctx, reader, refs)
			reader.done()
			if got.State != identity.Unavailable || got.DatabaseCDB.State != tc.want || (tc.err == nil && err != nil) || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatalf("failure/absence lost: %#v %v", got, err)
			}
		})
	}
}
