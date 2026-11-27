package identity_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
)

func TestReadBorrowedSampleIdentity(t *testing.T) {
	const statement = `SELECT VERSION_FULL AS VERSION_FULL,
    SYS_CONTEXT('USERENV','SESSION_USER') AS SESSION_USER,
    SYS_CONTEXT('USERENV','ISDBA') AS ISDBA,
    SYS_CONTEXT('USERENV','CON_NAME') AS CON_NAME,
    DATABASE_ROLE AS DATABASE_ROLE,
    OPEN_MODE AS OPEN_MODE,
    SYS_CONTEXT('USERENV','NETWORK_PROTOCOL') AS NETWORK_PROTOCOL
FROM V$INSTANCE CROSS JOIN V$DATABASE`
	columns := []string{"VERSION_FULL", "SESSION_USER", "ISDBA", "CON_NAME", "DATABASE_ROLE", "OPEN_MODE", "NETWORK_PROTOCOL"}
	row := []driver.Value{"23.26.1.0.0", "SYSTEM", "FALSE", "CDB$ROOT", "PRIMARY", "READ WRITE", "tcp "}
	wantIdentity := identity.BorrowedSampleIdentity{
		Version: "23.26.1.0.0", User: "SYSTEM", IsDBA: "FALSE", Container: "CDB$ROOT",
		DatabaseRole: "PRIMARY", OpenMode: "READ WRITE", Protocol: "tcp ",
	}
	nativeErr := errors.New("native query failure")
	fetchErr := errors.New("native fetch failure")
	closeErr := errors.New("native close failure")
	nullRow := append([]driver.Value(nil), row...)
	nullRow[5] = nil
	for _, tc := range []struct {
		name     string
		step     testsql.Step
		wantErr  error
		null     bool
		canceled bool
	}{
		{name: "all values", step: testsql.Step{Values: [][]driver.Value{row}}},
		{name: "no row", wantErr: sql.ErrNoRows},
		{name: "nullable projection", step: testsql.Step{Values: [][]driver.Value{nullRow}}, null: true},
		{name: "failed query", step: testsql.Step{QueryErr: nativeErr}, wantErr: nativeErr},
		{name: "failed fetch", step: testsql.Step{NextErr: fetchErr}, wantErr: fetchErr},
		{name: "failed close", step: testsql.Step{Values: [][]driver.Value{row}, CloseErr: closeErr}, wantErr: closeErr},
		{name: "canceled fetch", step: testsql.Step{Values: [][]driver.Value{row}}, wantErr: context.Canceled, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			step := tc.step
			step.SQL, step.Columns = statement, columns
			if tc.canceled {
				step.OnNext = func(int) { cancel() }
			}
			pool, state := testsql.New(t, step)
			conn, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			got, err := identity.ReadBorrowedSampleIdentity(ctx, conn)
			switch {
			case tc.null:
				if err == nil || !strings.Contains(err.Error(), "converting NULL to string") {
					t.Fatalf("NULL scan returned %v", err)
				}
			case tc.wantErr != nil:
				if err != tc.wantErr {
					t.Fatalf("read error = %v, want original %v", err, tc.wantErr)
				}
			case err != nil || got != wantIdentity:
				t.Fatalf("read = %+v, %v, want %+v", got, err, wantIdentity)
			}
			if err != nil && got != (identity.BorrowedSampleIdentity{}) {
				t.Fatalf("failed read exposed a partial identity: %+v", got)
			}
			calls := state.Calls()
			if len(calls) != 1 || calls[0].SQL != statement || len(calls[0].Args) != 0 {
				t.Fatalf("query calls = %+v, want one exact, unbound projection", calls)
			}
			wantClosed := 1
			if step.QueryErr != nil {
				wantClosed = 0
			}
			if state.Closed() != wantClosed {
				t.Fatalf("cursors closed = %d, want %d", state.Closed(), wantClosed)
			}
			if err := conn.Raw(func(any) error { return nil }); err != nil {
				t.Fatalf("identity reader retired or closed the caller's lease: %v", err)
			}
		})
	}
}

func TestReadBorrowedSampleIdentityRejectsMissingLease(t *testing.T) {
	got, err := identity.ReadBorrowedSampleIdentity(t.Context(), nil)
	if got != (identity.BorrowedSampleIdentity{}) || err != identity.ErrInvalidRequest {
		t.Fatalf("missing connection returned %+v, %v", got, err)
	}
}
