package oracleinfo_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/marcuoli/go-oracle-info/database"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-info/storage"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestRelocatedAllocationAndFileFacts(t *testing.T) {
	t.Run("SGA", func(t *testing.T) {
		checkRelocatedDataset(t, database.SGAAllocatedStatement, memory.SGAAllocatedStatement,
			database.SGAAllocatedJSON, memory.SGAAllocatedJSON,
			testsql.Step{Columns: []string{"VALUE"}, Values: [][]driver.Value{{"3.125"}}})
	})
	t.Run("PGA empty", func(t *testing.T) {
		checkRelocatedDataset(t, database.PGAAllocatedStatement, memory.PGAAllocatedStatement,
			database.PGAAllocatedJSON, memory.PGAAllocatedJSON, testsql.Step{Columns: []string{"VALUE"}})
		result, err := memory.DecodePGAAllocated(session.Result{Columns: []string{"VALUE"}}, memory.PGAAllocatedOptions{OracleMajor: 19})
		if err != nil || result.Rows == nil || len(result.Rows) != 0 {
			t.Fatalf("PGA lost verified empty result: %+v %v", result, err)
		}
	})
	t.Run("datafile size", func(t *testing.T) {
		checkRelocatedDataset(t, database.DatafileSizeStatement, storage.DatafileSizeStatement,
			database.DatafileSizeJSON, storage.DatafileSizeJSON,
			testsql.Step{Columns: []string{"VALUE"}, Values: [][]driver.Value{{nil}}})
	})
	t.Run("file names", func(t *testing.T) {
		checkRelocatedDataset(t, database.StorageStatement, storage.FileNamesStatement,
			database.StorageJSON, storage.FileNamesJSON,
			testsql.Step{Columns: []string{"FILE_NAME", "FILE_KIND"}, Values: [][]driver.Value{{"+DATA/name", "DATAFILE"}, {nil, "TEMPFILE"}}})
	})
}

func checkRelocatedDataset[Old, New ~struct{ OracleMajor int }](
	t *testing.T,
	oldStatement func(Old) (string, []any, error),
	newStatement func(New) (string, []any, error),
	oldJSON func(context.Context, rowset.Queryer, Old, rowset.Limits) (json.RawMessage, error),
	newJSON func(context.Context, rowset.Queryer, New, rowset.Limits) (json.RawMessage, error),
	step testsql.Step,
) {
	t.Helper()
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		options := struct{ OracleMajor int }{major}
		oldSQL, oldArgs, oldErr := oldStatement(Old(options))
		newSQL, newArgs, newErr := newStatement(New(options))
		supported := major == 19 || major == 21 || major == 23
		if (oldErr == nil) != supported || (newErr == nil) != supported || oldSQL != newSQL || !reflect.DeepEqual(oldArgs, newArgs) {
			t.Fatalf("major %d selection parity: %q %v vs %q %v", major, oldSQL, oldErr, newSQL, newErr)
		}
	}
	oldDB, oldState := testsql.New(t, step)
	newDB, newState := testsql.New(t, step)
	options := struct{ OracleMajor int }{19}
	prior, oldErr := oldJSON(t.Context(), oldDB, Old(options), rowset.Limits{})
	current, newErr := newJSON(t.Context(), newDB, New(options), rowset.Limits{})
	if oldErr != nil || newErr != nil || !bytes.Equal(prior, current) ||
		!reflect.DeepEqual(oldState.Calls(), newState.Calls()) || oldState.Closed() != 1 || newState.Closed() != 1 {
		t.Fatalf("JSON/cursor changed: %s %v vs %s %v", prior, oldErr, current, newErr)
	}
}

func TestRelocatedSGAComponents(t *testing.T) {
	for _, major := range []int{9, 10, 11, 12, 18, 19, 21, 23, 26} {
		legacy, oldArgs, oldErr := storage.SGAComponentsStatement(major, storage.InstanceFilter{Instances: []int64{2, 5}})
		current, newArgs, newErr := memory.SGAComponentsStatement(major, memory.InstanceFilter{Instances: []int64{2, 5}})
		admitted := major >= 11
		if (oldErr == nil) != admitted || (newErr == nil) != admitted || legacy != current ||
			!reflect.DeepEqual(oldArgs, newArgs) || (admitted && !reflect.DeepEqual(newArgs, []any{int64(2), int64(5)})) {
			t.Fatalf("components major %d selection parity: %q %v %v vs %q %v %v", major, legacy, oldArgs, oldErr, current, newArgs, newErr)
		}
	}
	cols := []string{"INST_ID", "POOL", "NAME", "BYTES", "CON_ID"}
	step := testsql.Step{Columns: cols, Values: [][]driver.Value{{int64(2), nil, "buffer_cache", int64(9007199254740993), nil}}}
	oldDB, oldState := testsql.New(t, step)
	newDB, newState := testsql.New(t, step)
	legacy, oldErr := storage.SGAComponentsJSON(t.Context(), oldDB, 19, storage.InstanceFilter{Instances: []int64{2}}, rowset.Limits{})
	current, newErr := memory.SGAComponentsJSON(t.Context(), newDB, 19, memory.InstanceFilter{Instances: []int64{2}}, rowset.Limits{})
	if oldErr != nil || newErr != nil || !bytes.Equal(legacy, current) ||
		!bytes.Contains(current, []byte("9007199254740993")) ||
		!reflect.DeepEqual(oldState.Calls(), newState.Calls()) || oldState.Closed() != 1 || newState.Closed() != 1 {
		t.Fatalf("component JSON/cursor changed: %s %v vs %s %v", legacy, oldErr, current, newErr)
	}
}

type ownershipReader func(context.Context, session.Statement) (session.Result, error)

func (r ownershipReader) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	return r(ctx, statement)
}

func TestRelocatedFactErrorsPreserveLegacyDomainAndCallerCause(t *testing.T) {
	ref, err := reference.NewReference("unit", "ownership", "allocated", 1)
	if err != nil {
		t.Fatal(err)
	}
	badResult := session.Result{Columns: []string{"wrong_column"}}
	for _, tc := range []struct {
		name, source string
		read         func(context.Context, ownershipReader) error
		foreign      error
	}{
		{"allocation", "V$SGA", func(ctx context.Context, r ownershipReader) error {
			_, err := database.ReadSGAAllocated(ctx, r, database.SGAAllocatedRequest{Options: database.SGAAllocatedOptions{OracleMajor: 19}, Reference: ref})
			return err
		}, memory.ErrInvalidObservedEvidence},
		{"file summary", "DBA_DATA_FILES", func(ctx context.Context, r ownershipReader) error {
			_, err := database.ReadDatafileSize(ctx, r, database.DatafileSizeRequest{Options: database.DatafileSizeOptions{OracleMajor: 19}, Reference: ref})
			return err
		}, storage.ErrInvalidEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			malformed := tc.read(t.Context(), func(context.Context, session.Statement) (session.Result, error) {
				return badResult, nil
			})
			var validation *database.DatasetError
			if !errors.As(malformed, &validation) || validation.Source != tc.source || !errors.Is(malformed, database.ErrInvalidEvidence) || errors.Is(malformed, tc.foreign) {
				t.Fatalf("legacy validation error changed: %v", malformed)
			}
			foreign := tc.read(t.Context(), func(context.Context, session.Statement) (session.Result, error) {
				return session.Result{}, tc.foreign
			})
			if !errors.Is(foreign, tc.foreign) || errors.Is(foreign, database.ErrInvalidEvidence) {
				t.Fatalf("caller failure was translated: %v", foreign)
			}
		})
	}

	_, err = storage.ReadSGAComponents(t.Context(), ownershipReader(func(context.Context, session.Statement) (session.Result, error) {
		return badResult, nil
	}), storage.SGARequest{OracleMajor: 19, Reference: ref})
	outer, ok := err.(*storage.Error)
	if !ok || outer.Source != "GV$SGASTAT" {
		t.Fatalf("legacy SGA read wrapper changed: %v", err)
	}
	var inner *storage.Error
	if !errors.As(outer.Unwrap(), &inner) || inner.Source != outer.Source || inner.Unwrap() != storage.ErrInvalidEvidence {
		t.Fatalf("legacy SGA decoder wrapper changed: %v", outer.Unwrap())
	}
}
