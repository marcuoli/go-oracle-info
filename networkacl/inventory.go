// Package networkacl selects distinct read-only ACL visibility facts.
package networkacl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Dataset identifies a distinct documented visibility/source responsibility.
type Dataset string

const (
	// DBAHostMappings selects admitted DBA host-to-ACL mappings.
	DBAHostMappings Dataset = "dba_host_mappings"
	// DBAHostPrivileges selects DBA ACL/ACE privilege facts, not effective access.
	DBAHostPrivileges Dataset = "dba_host_privileges"
	// DBAWalletMappings selects DBA wallet assignments, including the 11g mapping.
	DBAWalletMappings Dataset = "dba_wallet_mappings"
	// DBAWalletPrivileges selects DBA wallet ACE facts from 12c onward.
	DBAWalletPrivileges Dataset = "dba_wallet_privileges"
	// UserHostPrivileges selects the current user's visible host status only.
	UserHostPrivileges Dataset = "user_host_privileges"
	// UserWalletPrivileges selects the current user's visible wallet status only.
	UserWalletPrivileges Dataset = "user_wallet_privileges"
)

// Options admits one source/release; it changes no grants or session identity.
type Options struct {
	OracleMajor int
	Dataset     Dataset
}

// Reader supplies complete, bounded results and retains terminal/close failures.
// Its owner retains admission, deadlines, session lifetime and recovery policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request supplies a stable caller reference and finite result limits. Zero
// limits use shared safe defaults; the reader also needs transport/LOB budgets.
type Request struct {
	Options   Options
	Reference reference.Reference
	Limits    rowset.Limits
}

// Provenance retains the selected source, release and dictionary visibility.
// Successful empty records mean an empty admitted selection, never unavailable.
type Provenance struct {
	Source      string  `json:"source"`
	OracleMajor int     `json:"oracle_major"`
	Scope       string  `json:"scope"`
	Dataset     Dataset `json:"dataset"`
}

var (
	// ErrUnsupportedVersion rejects sources before Oracle 11g.
	ErrUnsupportedVersion = errors.New("Oracle dictionary facts require Oracle 11g or later")
	// ErrInvalidRequest rejects invalid scopes, limits, readers or references.
	ErrInvalidRequest = errors.New("invalid Oracle dictionary fact request")
	// ErrInvalidEvidence rejects malformed or partial dictionary observations.
	ErrInvalidEvidence = errors.New("invalid Oracle dictionary fact evidence")
)

// Error retains a private cause for errors.Is/As without formatting source data.
type Error struct {
	Operation string
	cause     error
}

func (e *Error) Error() string { return "Oracle dictionary " + e.Operation + " failed" }

func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql        string
	args       []any
	schema     []rowset.Column
	integers   []string
	provenance Provenance
}

func columns(result session.Result, plan selection) (map[string]int, error) {
	names := make([]string, len(plan.schema))
	for i, c := range plan.schema {
		names[i] = c.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	for _, row := range result.Rows {
		for _, v := range row {
			if !utf8.ValidString(v.Text) || (v.Null && v.Text != "") {
				return nil, ErrInvalidEvidence
			}
		}
		for _, name := range plan.integers {
			v := row[index[name]]
			if !v.Null && !numeric.IsSignedInteger(v.Text) {
				return nil, ErrInvalidEvidence
			}
		}
	}
	return index, nil
}

func query(ctx context.Context, reader Reader, request Request, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return session.Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return session.Result{}, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{Operation: "query", cause: err}
	}
	if len(result.Rows) > limits.MaxRows {
		return session.Result{}, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	// Count every retained value, including late values, before publishing facts.
	retained := 0
	for _, row := range result.Rows {
		for _, v := range row {
			if len(v.Text) > limits.MaxBytes-retained {
				return session.Result{}, &Error{Operation: "budget", cause: rowset.ErrCapacity}
			}
			retained += len(v.Text)
		}
	}
	return result, nil
}

func readJSON(ctx context.Context, q rowset.Queryer, plan selection, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql, plan.args...), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: plan.provenance,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(result session.Result) error {
			_, err := columns(result, plan)
			return err
		}),
	})
	return payload, projection.WrapColumnEnvelopeError(err, func(operation string, cause error) error {
		return &Error{Operation: operation, cause: cause}
	})
}

// Record retains one source record. Nil fields were not projected by this source;
// nonnil SQL NULL, raw legacy flags and modern flags remain distinct.
// Mappings and privileges do not establish complete effective network access.
type Record struct {
	Host              *session.Value
	LowerPort         *session.Value
	UpperPort         *session.Value
	ACL               *session.Value
	ACLID             *session.Value
	ACLOwner          *session.Value
	WalletPath        *session.Value
	ACEOrder          *session.Value
	StartDate         *session.Value
	EndDate           *session.Value
	GrantType         *session.Value
	InvertedPrincipal *session.Value
	Principal         *session.Value
	PrincipalType     *session.Value
	Privilege         *session.Value
	Status            *session.Value
}

// Result is a complete observation of exactly one admitted ACL dataset.
type Result struct {
	Provenance Provenance
	Records    []Record
}

// ErrUnsupportedCapability rejects unavailable release/source combinations.
var ErrUnsupportedCapability = errors.New("ACL dataset requires Oracle 12c or later")

// Statement selects reviewed aliases for one source, without privilege fallback.
func Statement(options Options) (string, []any, error) {
	p, e := selectACL(options)
	return p.sql, p.args, e
}

func selectACL(options Options) (selection, error) {
	if options.OracleMajor < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	source := ""
	scope := "connected_container_dba_dictionary"
	names := []string{}
	order := ""
	modern := options.OracleMajor >= 12
	switch options.Dataset {
	case DBAHostMappings:
		source = "DBA_NETWORK_ACLS"
		names = []string{"HOST", "LOWER_PORT", "UPPER_PORT", "ACL", "ACLID"}
		if modern {
			source = "DBA_HOST_ACLS"
			names = append(names, "ACL_OWNER")
		}
		order = "HOST,LOWER_PORT,UPPER_PORT,ACL"
	case DBAHostPrivileges:
		if modern {
			source = "DBA_HOST_ACES"
			names = []string{"HOST", "LOWER_PORT", "UPPER_PORT", "ACE_ORDER", "START_DATE", "END_DATE", "GRANT_TYPE", "INVERTED_PRINCIPAL", "PRINCIPAL", "PRINCIPAL_TYPE", "PRIVILEGE"}
			order = "HOST,LOWER_PORT,UPPER_PORT,ACE_ORDER,PRIVILEGE"
		} else {
			source = "DBA_NETWORK_ACL_PRIVILEGES"
			names = []string{"ACL", "ACLID", "PRINCIPAL", "PRIVILEGE", "GRANT_TYPE", "INVERTED_PRINCIPAL", "START_DATE", "END_DATE"}
			order = "ACL,PRINCIPAL,PRIVILEGE,START_DATE,END_DATE"
		}
	case DBAWalletMappings:
		source = "DBA_WALLET_ACLS"
		names = []string{"WALLET_PATH", "ACL", "ACLID"}
		if modern {
			names = append(names, "ACL_OWNER")
		}
		order = "WALLET_PATH,ACL"
	case DBAWalletPrivileges:
		if !modern {
			return selection{}, ErrUnsupportedCapability
		}
		source = "DBA_WALLET_ACES"
		names = []string{"WALLET_PATH", "ACE_ORDER", "START_DATE", "END_DATE", "GRANT_TYPE", "INVERTED_PRINCIPAL", "PRINCIPAL", "PRINCIPAL_TYPE", "PRIVILEGE"}
		order = "WALLET_PATH,ACE_ORDER,PRIVILEGE"
	case UserHostPrivileges:
		scope = "current_user_visible_privilege_status"
		source = "USER_NETWORK_ACL_PRIVILEGES"
		if modern {
			source = "USER_HOST_ACES"
		}
		names = []string{"HOST", "LOWER_PORT", "UPPER_PORT", "PRIVILEGE", "STATUS"}
		order = "HOST,LOWER_PORT,UPPER_PORT,PRIVILEGE"
	case UserWalletPrivileges:
		if !modern {
			return selection{}, ErrUnsupportedCapability
		}
		scope = "current_user_visible_privilege_status"
		source = "USER_WALLET_ACES"
		names = []string{"WALLET_PATH", "PRIVILEGE", "STATUS"}
		order = "WALLET_PATH,PRIVILEGE"
	default:
		return selection{}, ErrInvalidRequest
	}
	expressions := make([]string, len(names))
	schema := make([]rowset.Column, len(names))
	integers := []string{}
	for i, name := range names {
		expression := "t." + name
		switch name {
		case "ACLID":
			expression = "RAWTOHEX(t.ACLID)"
		case "LOWER_PORT", "UPPER_PORT", "ACE_ORDER":
			expression = "TO_CHAR(t." + name + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
			integers = append(integers, name)
		case "START_DATE", "END_DATE":
			expression = "TO_CHAR(t." + name + ", 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9', 'NLS_DATE_LANGUAGE=American')"
		case "GRANT_TYPE":
			if !modern {
				expression = "t.IS_GRANT"
			}
		case "INVERTED_PRINCIPAL":
			if !modern {
				expression = "t.INVERT"
			}
		}
		expressions[i] = expression + " AS " + name
		schema[i] = rowset.Column{Name: name, Kind: rowset.Text}
	}
	return selection{sql: "SELECT " + strings.Join(expressions, ", ") + " FROM " + source + " t ORDER BY " + order, args: []any{}, schema: schema, integers: integers, provenance: Provenance{Source: source, OracleMajor: options.OracleMajor, Scope: scope, Dataset: options.Dataset}}, nil
}

// Decode validates every row; only an admitted Reader can attest transport EOF.
// Legacy IS_GRANT/INVERT remain raw under the documented unified aliases.
func Decode(result session.Result, options Options) (Result, error) {
	p, e := selectACL(options)
	if e != nil {
		return Result{}, e
	}
	return decode(result, p)
}

func decode(result session.Result, plan selection) (Result, error) {
	index, err := columns(result, plan)
	if err != nil {
		return Result{}, &Error{Operation: "decode", cause: err}
	}
	records := make([]Record, len(result.Rows))
	for i, row := range result.Rows {
		if j, selected := index["HOST"]; selected {
			v := row[j]
			records[i].Host = &v
		}
		if j, selected := index["LOWER_PORT"]; selected {
			v := row[j]
			records[i].LowerPort = &v
		}
		if j, selected := index["UPPER_PORT"]; selected {
			v := row[j]
			records[i].UpperPort = &v
		}
		if j, selected := index["ACL"]; selected {
			v := row[j]
			records[i].ACL = &v
		}
		if j, selected := index["ACLID"]; selected {
			v := row[j]
			records[i].ACLID = &v
		}
		if j, selected := index["ACL_OWNER"]; selected {
			v := row[j]
			records[i].ACLOwner = &v
		}
		if j, selected := index["WALLET_PATH"]; selected {
			v := row[j]
			records[i].WalletPath = &v
		}
		if j, selected := index["ACE_ORDER"]; selected {
			v := row[j]
			records[i].ACEOrder = &v
		}
		if j, selected := index["START_DATE"]; selected {
			v := row[j]
			records[i].StartDate = &v
		}
		if j, selected := index["END_DATE"]; selected {
			v := row[j]
			records[i].EndDate = &v
		}
		if j, selected := index["GRANT_TYPE"]; selected {
			v := row[j]
			records[i].GrantType = &v
		}
		if j, selected := index["INVERTED_PRINCIPAL"]; selected {
			v := row[j]
			records[i].InvertedPrincipal = &v
		}
		if j, selected := index["PRINCIPAL"]; selected {
			v := row[j]
			records[i].Principal = &v
		}
		if j, selected := index["PRINCIPAL_TYPE"]; selected {
			v := row[j]
			records[i].PrincipalType = &v
		}
		if j, selected := index["PRIVILEGE"]; selected {
			v := row[j]
			records[i].Privilege = &v
		}
		if j, selected := index["STATUS"]; selected {
			v := row[j]
			records[i].Status = &v
		}
	}
	return Result{Provenance: plan.provenance, Records: records}, nil
}

// Read executes one complete admitted read, with no privilege fallback or prefix.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	plan, err := selectACL(request.Options)
	if err != nil {
		return Result{}, err
	}
	r, err := query(ctx, reader, request, plan)
	if err != nil {
		return Result{}, err
	}
	facts, de := decode(r, plan)
	if err = errors.Join(de, ctx.Err()); err != nil {
		return Result{}, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the caller-native cursor; the caller owns Next/Scan/Err/Close,
// finite budgets and session affinity. No identity or privilege changes occur.
func Rows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, e := selectACL(options)
	if e != nil {
		return nil, e
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// JSON independently consumes/finalizes the source with finite row/byte budgets.
// It publishes no bytes after query, terminal, close or cancellation failure.
func JSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, e := selectACL(options)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}

// InventoryStatement is the named form of Statement.
func InventoryStatement(options Options) (string, []any, error) {
	return Statement(options)
}

// DecodeInventory is the named form of Decode.
func DecodeInventory(result session.Result, options Options) (Result, error) {
	return Decode(result, options)
}

// ReadInventory is the named form of Read.
func ReadInventory(ctx context.Context, reader Reader, request Request) (Result, error) {
	return Read(ctx, reader, request)
}

// InventoryRows is the named form of Rows.
func InventoryRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	return Rows(ctx, q, options)
}

// InventoryJSON is the named form of JSON.
func InventoryJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options, limits)
}
