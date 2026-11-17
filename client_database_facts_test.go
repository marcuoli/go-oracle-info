package oracleinfo_test

import (
	"database/sql/driver"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
)

func TestEncryptionFacadeChoosesActualConnectedScopeAndCursor(t *testing.T) {
	for _, tc := range []struct {
		name, containerName, containerID string
		root                             bool
	}{
		{"root", "CDB$ROOT", "1", true}, {"pdb", "APP", "3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _ := database.EncryptionStatement(19, tc.root, database.ColumnEncryption)
			db, state := testsql.New(t,
				testsql.Step{SQL: legacySQL, Columns: []string{"VERSION"}, Values: [][]driver.Value{{"19.0.0.0.0"}}},
				testsql.Step{SQL: identity.ConnectedArchitectureSQL, Columns: []string{"CDB", "CONTAINER_ID", "CONTAINER_NAME"}, Values: [][]driver.Value{{"YES", tc.containerID, tc.containerName}}},
				testsql.Step{SQL: sql, Columns: []string{"ENCRYPTED_COUNT", "UNKNOWN_COUNT"}, Values: [][]driver.Value{{"0", "0"}}})
			spy := &nativeSpy{db: db}
			rows, err := info.New(spy).EncryptionCounts(t.Context(), database.ColumnEncryption)
			if err != nil || rows != spy.last {
				t.Fatal("not the actual caller-native cursor", err)
			}
			if !rows.Next() {
				t.Fatal("count row missing", rows.Err())
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			if len(state.Calls()) != 3 || state.Closed() != 3 {
				t.Fatal("admission leaked or added a connection", state.Calls(), state.Closed())
			}
		})
	}
	db, state := testsql.New(t)
	if _, err := info.New(db).EncryptionCounts(t.Context(), database.EncryptionKind("invalid")); err == nil || len(state.Calls()) != 0 {
		t.Fatal("invalid kind queried")
	}
}
