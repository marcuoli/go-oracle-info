package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/asm"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/redo"
	"github.com/marcuoli/go-oracle-info/storage"
	"strings"
	"testing"
	"time"
)

func TestStorageFacadeReturnsActualNativeRows(t *testing.T) {
	history := info.RedoHistoryFilter{From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)}
	methods := []struct {
		name   string
		source string
		call   func(*info.Client) (*sql.Rows, error)
	}{
		{"groups", "GV$LOG", func(c *info.Client) (*sql.Rows, error) { return c.RedoGroups(t.Context(), info.RedoFilter{}) }},
		{"members", "GV$LOGFILE", func(c *info.Client) (*sql.Rows, error) { return c.RedoMembers(t.Context(), info.RedoFilter{}) }},
		{"history", "GV$LOG_HISTORY", func(c *info.Client) (*sql.Rows, error) { return c.RedoHistory(t.Context(), history) }},
		{"archives", "GV$ARCHIVED_LOG", func(c *info.Client) (*sql.Rows, error) { return c.ArchivedLogs(t.Context(), history) }},
		{"tablespaces", "DBA_TABLESPACES", func(c *info.Client) (*sql.Rows, error) { return c.Tablespaces(t.Context()) }},
		{"datafiles", "DBA_DATA_FILES", func(c *info.Client) (*sql.Rows, error) { return c.Datafiles(t.Context()) }},
		{"tempfiles", "DBA_TEMP_FILES", func(c *info.Client) (*sql.Rows, error) {
			return c.Tempfiles(t.Context(), info.TempfileOptions{IncludeLocal: true})
		}},
		{"temp-free", "DBA_TEMP_FREE_SPACE", func(c *info.Client) (*sql.Rows, error) { return c.TempFreeSpace(t.Context()) }},
		{"free", "DBA_FREE_SPACE", func(c *info.Client) (*sql.Rows, error) { return c.FreeExtents(t.Context()) }},
		{"segments", "DBA_SEGMENTS", func(c *info.Client) (*sql.Rows, error) { return c.Segments(t.Context()) }},
		{"long", "DBA_TAB_COLUMNS", func(c *info.Client) (*sql.Rows, error) { return c.LongColumns(t.Context()) }},
		{"default-temp", "DATABASE_PROPERTIES", func(c *info.Client) (*sql.Rows, error) { return c.DefaultTemporary(t.Context()) }},
		{"temp-groups", "DBA_TABLESPACE_GROUPS", func(c *info.Client) (*sql.Rows, error) { return c.TemporaryGroups(t.Context()) }},
		{"sga", "GV$SGASTAT", func(c *info.Client) (*sql.Rows, error) {
			return c.SGAComponents(t.Context(), info.SGAComponentFilter{})
		}},
		{"asm", "GV$ASM_CLIENT", func(c *info.Client) (*sql.Rows, error) {
			return c.ASMClients(t.Context(), info.ASMClientFilter{SourceRole: asm.DatabaseInstance})
		}},
	}
	for _, tc := range methods {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, append(versionSteps("19"), testsql.Step{Columns: []string{"BORROWED"}, Values: [][]driver.Value{{"usable"}}})...)
			spy := &nativeSpy{db: db}
			rows, err := tc.call(info.New(spy))
			if err != nil {
				t.Fatal(err)
			}
			if rows != spy.last {
				t.Fatal("fabricated or wrapped native cursor")
			}
			if !rows.Next() {
				t.Fatalf("cursor closed early: %v", rows.Err())
			}
			if err = rows.Close(); err != nil {
				t.Fatal(err)
			}
			calls := state.Calls()
			last := calls[len(calls)-1]
			if !containsStorageSource(last.SQL, tc.source) {
				t.Fatalf("wrong source/version wiring: %s", last.SQL)
			}
		})
	}
}
func containsStorageSource(sql, source string) bool {
	return strings.Contains(sql, " FROM "+source+" v")
}

func TestStorageFacadeJSONAndRejectedSelection(t *testing.T) {
	statement, err := storage.DefaultTemporaryStatement(19)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{SQL: statement, Columns: []string{"SOURCE_INST_ID", "PROPERTY_NAME", "PROPERTY_VALUE", "DESCRIPTION", "CON_ID"}, Values: [][]driver.Value{{"1", "DEFAULT_TEMP_TABLESPACE", nil, "raw description", "3"}}}
	db, _ := testsql.New(t, append(versionSteps("19"), step)...)
	payload, err := info.New(db).DefaultTemporaryJSON(t.Context(), info.JSONLimits{MaxRows: 1, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance storage.Provenance
		Data       struct{ Rows [][]any }
	}
	if err = json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.Complete || len(envelope.Data.Rows) != 1 || envelope.Data.Rows[0][2] != nil {
		t.Fatalf("lost native NULL/completion: %s", payload)
	}
	empty, state := testsql.New(t)
	c := info.New(empty)
	if _, err = c.RedoHistory(context.Background(), redo.HistoryFilter{}); err == nil {
		t.Fatal("unbounded history admitted")
	}
	if _, err = c.ASMClients(context.Background(), asm.Filter{}); err == nil {
		t.Fatal("unadmitted source role")
	}
	if _, err = c.RedoGroups(context.Background(), redo.Filter{Instances: []int64{1, 1}}); err == nil {
		t.Fatal("duplicate scope")
	}
	if _, err = c.DatafilesJSON(context.Background(), info.JSONLimits{MaxRows: -1}); err == nil {
		t.Fatal("invalid budget")
	}
	if len(state.Calls()) != 0 {
		t.Fatalf("invalid request queried source/version: %+v", state.Calls())
	}
}
