package networkacl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

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
