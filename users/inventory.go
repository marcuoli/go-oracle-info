package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// User retains account metadata only; password hashes, verifiers, external identities and credentials are never projected.
// Numeric fields retain exact decimal text and SQL NULL. Optional pointers mean
// the release projection omitted a column; nonnil Null means projected SQL NULL.
type User struct {
	Username                     session.Value
	UserID                       session.Value
	AccountStatus                session.Value
	LockDate                     session.Value
	ExpiryDate                   session.Value
	DefaultTablespace            session.Value
	TemporaryTablespace          session.Value
	Created                      session.Value
	Profile                      session.Value
	InitialResourceConsumerGroup session.Value
	AuthenticationType           session.Value
	EditionsEnabled              session.Value
	Common                       *session.Value
	OracleMaintained             *session.Value
	Inherited                    *session.Value
}

// Result is one complete selected dictionary observation, in source order.
type Result struct {
	Provenance Provenance
	Records    []User
}

// Statement selects explicit aliases with bound, exact scope. No source fallback,
// object execution, account mutation or collection retry occurs here.
func Statement(options Options) (string, []any, error) {
	p, e := inventoryPlan(options)
	return p.sql, p.args, e
}
func inventoryPlan(options Options) (selection, error) {
	if err := validate(options); err != nil {
		return selection{}, err
	}
	if len(options.Owners) > 0 {
		return selection{}, ErrInvalidRequest
	}
	expressions := []string{}
	schema := []rowset.Column{}
	integers := []string{}
	expressions = append(expressions, "t.USERNAME AS USERNAME")
	schema = append(schema, rowset.Column{Name: "USERNAME", Kind: rowset.Text})
	expressions = append(expressions, "TO_CHAR(t.USER_ID, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS USER_ID")
	schema = append(schema, rowset.Column{Name: "USER_ID", Kind: rowset.Text})
	integers = append(integers, "USER_ID")
	expressions = append(expressions, "t.ACCOUNT_STATUS AS ACCOUNT_STATUS")
	schema = append(schema, rowset.Column{Name: "ACCOUNT_STATUS", Kind: rowset.Text})
	expressions = append(expressions, datetime.GregorianDate("t.LOCK_DATE")+" AS LOCK_DATE")
	schema = append(schema, rowset.Column{Name: "LOCK_DATE", Kind: rowset.Text})
	expressions = append(expressions, datetime.GregorianDate("t.EXPIRY_DATE")+" AS EXPIRY_DATE")
	schema = append(schema, rowset.Column{Name: "EXPIRY_DATE", Kind: rowset.Text})
	expressions = append(expressions, "t.DEFAULT_TABLESPACE AS DEFAULT_TABLESPACE")
	schema = append(schema, rowset.Column{Name: "DEFAULT_TABLESPACE", Kind: rowset.Text})
	expressions = append(expressions, "t.TEMPORARY_TABLESPACE AS TEMPORARY_TABLESPACE")
	schema = append(schema, rowset.Column{Name: "TEMPORARY_TABLESPACE", Kind: rowset.Text})
	expressions = append(expressions, datetime.GregorianDate("t.CREATED")+" AS CREATED")
	schema = append(schema, rowset.Column{Name: "CREATED", Kind: rowset.Text})
	expressions = append(expressions, "t.PROFILE AS PROFILE")
	schema = append(schema, rowset.Column{Name: "PROFILE", Kind: rowset.Text})
	expressions = append(expressions, "t.INITIAL_RSRC_CONSUMER_GROUP AS INITIAL_RSRC_CONSUMER_GROUP")
	schema = append(schema, rowset.Column{Name: "INITIAL_RSRC_CONSUMER_GROUP", Kind: rowset.Text})
	expressions = append(expressions, "t.AUTHENTICATION_TYPE AS AUTHENTICATION_TYPE")
	schema = append(schema, rowset.Column{Name: "AUTHENTICATION_TYPE", Kind: rowset.Text})
	expressions = append(expressions, "t.EDITIONS_ENABLED AS EDITIONS_ENABLED")
	schema = append(schema, rowset.Column{Name: "EDITIONS_ENABLED", Kind: rowset.Text})
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.COMMON AS COMMON")
		schema = append(schema, rowset.Column{Name: "COMMON", Kind: rowset.Text})
	}
	if options.OracleMajor >= 12 {
		expressions = append(expressions, "t.ORACLE_MAINTAINED AS ORACLE_MAINTAINED")
		schema = append(schema, rowset.Column{Name: "ORACLE_MAINTAINED", Kind: rowset.Text})
	}
	if options.OracleMajor >= 18 {
		expressions = append(expressions, "t.INHERITED AS INHERITED")
		schema = append(schema, rowset.Column{Name: "INHERITED", Kind: rowset.Text})
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + "DBA_USERS t"
	args := []any{}
	statement, args = scopeFilter(statement, args, "t.USERNAME", options.Names)
	statement += " ORDER BY t.USERNAME"
	return selection{nameColumn: "USERNAME", ownerColumn: "", sql: statement, args: args, schema: schema, integers: integers, provenance: Provenance{Source: "DBA_USERS", OracleMajor: options.OracleMajor, Scope: "connected_container_dba_dictionary", Owners: append([]string(nil), options.Owners...), Names: append([]string(nil), options.Names...)}}, nil
}

// Decode validates every row before publishing any records. It cannot prove
// transport completion; use Read with an admitted complete Reader.
func Decode(result session.Result, options Options) (Result, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return Result{}, err
	}
	return decodeInventory(result, plan)
}
func decodeInventory(result session.Result, plan selection) (Result, error) {
	index, err := columns(result, plan)
	if err != nil {
		return Result{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]User, len(result.Rows))
	for i, row := range result.Rows {

		records[i] = User{Username: row[index["USERNAME"]], UserID: row[index["USER_ID"]], AccountStatus: row[index["ACCOUNT_STATUS"]], LockDate: row[index["LOCK_DATE"]], ExpiryDate: row[index["EXPIRY_DATE"]], DefaultTablespace: row[index["DEFAULT_TABLESPACE"]], TemporaryTablespace: row[index["TEMPORARY_TABLESPACE"]], Created: row[index["CREATED"]], Profile: row[index["PROFILE"]], InitialResourceConsumerGroup: row[index["INITIAL_RSRC_CONSUMER_GROUP"]], AuthenticationType: row[index["AUTHENTICATION_TYPE"]], EditionsEnabled: row[index["EDITIONS_ENABLED"]]}
		if position, selected := index["COMMON"]; selected {
			value := row[position]
			records[i].Common = &value
		}
		if position, selected := index["ORACLE_MAINTAINED"]; selected {
			value := row[position]
			records[i].OracleMaintained = &value
		}
		if position, selected := index["INHERITED"]; selected {
			value := row[position]
			records[i].Inherited = &value
		}
	}
	return Result{Provenance: plan.provenance, Records: records}, nil
}

// Read executes once, checks finite budgets and post-query cancellation,
// and returns no successful prefix after query, terminal or decode failure.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	plan, err := inventoryPlan(request.Options)
	if err != nil {
		return Result{}, err
	}
	result, err := query(ctx, reader, request, plan)
	if err != nil {
		return Result{}, err
	}
	facts, de := decodeInventory(result, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return Result{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the actual caller-owned native cursor, without row truncation.
// The caller owns Next/Scan/Err/Close, budgets and source/session affinity.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, plan.sql, plan.args...)
}

// JSON independently consumes the same selection through shared rowset
// lifecycle and finite encoded-byte/row budgets. No bytes are returned on failure.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := inventoryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}
