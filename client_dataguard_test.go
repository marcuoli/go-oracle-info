package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"testing"
)

func TestDataGuardFacadeReturnsActualNativeCursors(t *testing.T) {
	methods := []struct {
		name string
		call func(*info.Client) (*sql.Rows, error)
	}{
		{"context", func(c *info.Client) (*sql.Rows, error) { return c.DataGuardDatabaseContext(context.Background()) }},
		{"standby", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardStandbyLogs(context.Background(), info.DataGuardFilter{Instances: []int64{7}})
		}},
		{"processes", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardProcesses(context.Background(), info.DataGuardFilter{})
		}},
		{"recovery", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardRecoveryProgress(context.Background(), info.DataGuardFilter{})
		}},
		{"gaps", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardArchiveGaps(context.Background(), info.DataGuardFilter{})
		}},
		{"dest", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardArchiveDestinations(context.Background(), info.DataGuardFilter{})
		}},
		{"status", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardArchiveDestinationStatus(context.Background(), info.DataGuardFilter{})
		}},
		{"progress", func(c *info.Client) (*sql.Rows, error) {
			return c.DataGuardArchiveProgress(context.Background(), info.DataGuardArchiveProgressOptions{ResetlogsChange: "123", ResetlogsTime: "2026-11-18T01:02:03"})
		}},
	}
	for _, tc := range methods {
		t.Run(tc.name, func(t *testing.T) {
			db, state := testsql.New(t, append(versionSteps("23"), testsql.Step{Columns: []string{"NATIVE"}, Values: [][]driver.Value{{"alive"}}})...)
			spy := &nativeSpy{db: db}
			rows, err := tc.call(info.New(spy))
			if err != nil {
				t.Fatal(err)
			}
			if rows != spy.last || !rows.Next() {
				t.Fatalf("borrowed cursor changed or closed: %v", rows.Err())
			}
			if err = rows.Close(); err != nil {
				t.Fatal(err)
			}
			if len(state.Calls()) != 3 {
				t.Fatalf("unexpected version/cursor queries: %d", len(state.Calls()))
			}
		})
	}
}
func TestDataGuardFacadeRejectsInvalidAdmissionBeforeVersionQuery(t *testing.T) {
	db, state := testsql.New(t)
	c := info.New(db)
	if _, err := c.DataGuardStandbyLogs(t.Context(), info.DataGuardFilter{Instances: []int64{-1}}); err == nil {
		t.Fatal("invalid filter accepted")
	}
	if _, err := c.DataGuardArchiveProgress(t.Context(), info.DataGuardArchiveProgressOptions{ResetlogsChange: "123"}); err == nil {
		t.Fatal("missing incarnation time accepted")
	}
	if _, err := c.DataGuardProcessesJSON(t.Context(), info.DataGuardFilter{}, info.JSONLimits{MaxRows: -1}); err == nil {
		t.Fatal("negative limits accepted")
	}
	if len(state.Calls()) != 0 {
		t.Fatalf("invalid admission performed SQL: %d", len(state.Calls()))
	}
}
