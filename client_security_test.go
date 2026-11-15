package oracleinfo_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	info "github.com/marcuoli/go-oracle-info"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/metadata"
	"github.com/marcuoli/go-oracle-info/networkacl"
	"github.com/marcuoli/go-oracle-info/sequences"
)

func TestSecurityFacadesReturnExactCallerNativeCursor(t *testing.T) {
	methods := map[string]func(*info.Client, context.Context) (*sql.Rows, error){
		"users": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.Users(ctx, info.DictionaryFilter{})
		},
		"account-count": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.UserAccountCount(ctx, info.DictionaryFilter{})
		},
		"tables": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.TableMetadata(ctx, info.DictionaryFilter{})
		},
		"sequences": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.Sequences(ctx, info.DictionaryFilter{Owners: []string{"APP"}})
		},
		"directories": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.Directories(ctx, info.DictionaryFilter{})
		},
		"links": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.DatabaseLinks(ctx, info.DictionaryFilter{})
		},
		"triggers": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.Triggers(ctx, info.DictionaryFilter{})
		},
		"direct": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.DirectObjectGrants(ctx, info.DictionaryFilter{})
		},
		"columns": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.DirectColumnGrants(ctx, info.DictionaryFilter{})
		},
		"roles": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.RoleObjectGrants(ctx, info.DictionaryFilter{})
		},
		"profiles": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.SQLProfiles(ctx, info.DictionaryFilter{})
		},
		"baselines": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.SQLPlanBaselines(ctx, info.DictionaryFilter{})
		},
		"acls": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.NetworkACL(ctx, networkacl.DBAHostMappings)
		},
		"ddl": func(c *info.Client, ctx context.Context) (*sql.Rows, error) {
			return c.MetadataDDL(ctx, info.MetadataOptions{Kind: metadata.Trigger, Name: "T", Owner: "SYS"})
		},
	}
	for name, method := range methods {
		t.Run(name, func(t *testing.T) {
			db, state := testsql.New(t, append(versionSteps("19"), testsql.Step{Columns: []string{"RAW"}, Values: [][]driver.Value{{"x"}}})...)
			spy := &nativeSpy{db: db}
			rows, err := method(info.New(spy), t.Context())
			if err != nil || rows == nil || rows != spy.last || state.Closed() != 2 {
				t.Fatal(rows, err, state.Closed())
			}
			if !rows.Next() {
				t.Fatal(rows.Err())
			}
			if err = rows.Close(); err != nil || state.Closed() != 3 {
				t.Fatal(err, state.Closed())
			}
			if name == "sequences" {
				want, args, _ := sequences.Statement(sequences.Options{OracleMajor: 19, Owners: []string{"APP"}})
				calls := state.Calls()
				last := calls[len(calls)-1]
				if want != last.SQL || len(last.Args) != len(args) || last.Args[0].Value != "APP" {
					t.Fatal(last)
				}
			}
		})
	}
}
func TestSecurityFacadeSourceAndJSONIsolation(t *testing.T) {
	step := testsql.Step{Columns: []string{"HOST", "LOWER_PORT", "UPPER_PORT", "PRIVILEGE", "STATUS"}, Values: [][]driver.Value{{"*.invalid", nil, nil, "connect", "DENIED"}}}
	db, state := testsql.New(t, append(versionSteps("11"), step)...)
	payload, err := info.New(db).NetworkACLJSON(t.Context(), networkacl.UserHostPrivileges)
	if err != nil || !json.Valid(payload) || !strings.Contains(string(payload), "USER_NETWORK_ACL_PRIVILEGES") || !strings.Contains(string(payload), "current_user_visible_privilege_status") || state.Closed() != 2 {
		t.Fatal(string(payload), err, state.Closed())
	}
	closeErr := errors.New("private-close")
	step.CloseErr = closeErr
	db, _ = testsql.New(t, append(versionSteps("19"), step)...)
	payload, err = info.New(db).NetworkACLJSON(t.Context(), networkacl.UserHostPrivileges)
	if !errors.Is(err, closeErr) || payload != nil || strings.Contains(err.Error(), closeErr.Error()) {
		t.Fatal(string(payload), err)
	}
}
func TestSecurityFacadeInvalidFiltersAndLimitsDoNotQuery(t *testing.T) {
	db, state := testsql.New(t)
	c := info.New(db)
	calls := []func() error{
		func() error { _, e := c.Users(t.Context(), info.DictionaryFilter{Owners: []string{"SYS"}}); return e },
		func() error { _, e := c.Sequences(t.Context(), info.DictionaryFilter{Names: []string{""}}); return e },
		func() error { _, e := c.NetworkACL(t.Context(), "unknown"); return e },
		func() error {
			_, e := c.MetadataDDL(t.Context(), info.MetadataOptions{Kind: "USER", Name: "SYS"})
			return e
		},
		func() error {
			_, e := c.DirectoriesJSON(t.Context(), info.DictionaryFilter{}, info.JSONLimits{MaxRows: -1})
			return e
		},
		func() error {
			_, e := c.SQLProfilesJSON(t.Context(), info.DictionaryFilter{}, info.JSONLimits{}, info.JSONLimits{})
			return e
		},
	}
	for _, call := range calls {
		if err := call(); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if len(state.Calls()) != 0 {
		t.Fatal("invalid request queried", state.Calls())
	}
}
