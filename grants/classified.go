package grants

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ClassifiedOptions selects exact owners and named grantees in the current
// dictionary. OracleRelease optionally supplies the observed five-part numeric
// release, with the same major as Options. With major-only Oracle 12 evidence,
// projections omit flags introduced after 12.1.0.1.
// An exact PUBLIC entry in Names explicitly includes the special PUBLIC grantee.
type ClassifiedOptions struct {
	Options
	OracleRelease string
	// ApplicationPublicOnly requires exact PUBLIC as the only named scope.
	// It selects object owners or granted roles marked ORACLE_MAINTAINED=N.
	ApplicationPublicOnly bool
}

// ClassifiedRequest configures a bounded read through a caller-owned reader.
type ClassifiedRequest struct {
	Options   ClassifiedOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

type grantRelease struct {
	common         bool
	delegate       bool
	inherited      bool
	roleMaintained bool
}

func classifiedRelease(options ClassifiedOptions) (grantRelease, error) {
	if err := validate(options.Options); err != nil {
		return grantRelease{}, err
	}
	switch options.OracleMajor {
	case 11, 12, 18, 19, 21, 23:
	default:
		return grantRelease{}, ErrUnsupportedVersion
	}
	current := grantRelease{
		common:         options.OracleMajor >= 12,
		delegate:       options.OracleMajor >= 18,
		inherited:      options.OracleMajor >= 18,
		roleMaintained: options.OracleMajor >= 18,
	}
	if options.OracleRelease == "" {
		return current, nil
	}
	parts := strings.Split(options.OracleRelease, ".")
	if len(parts) != 5 {
		return grantRelease{}, ErrInvalidRequest
	}
	var numbers [5]int
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 || strconv.Itoa(value) != part {
			return grantRelease{}, ErrInvalidRequest
		}
		numbers[i] = value
	}
	if numbers[0] != options.OracleMajor {
		return grantRelease{}, ErrInvalidRequest
	}
	if options.OracleMajor == 12 {
		// 12.1.0.2 added DELEGATE_OPTION and 12.2.0.1 added INHERITED.
		switch numbers[1] {
		case 1:
			current.delegate = numbers[2] > 0 || numbers[3] >= 2
			current.roleMaintained = numbers[2] > 0 || numbers[3] >= 2
		case 2:
			current.delegate = true
			current.roleMaintained = numbers[2] > 0 || numbers[3] >= 1
			current.inherited = numbers[2] > 0 || numbers[3] >= 1
		default:
			return grantRelease{}, ErrUnsupportedVersion
		}
	}
	return current, nil
}

type grantColumn struct {
	expression string
	alias      string
}

func rawGrantColumns(names ...string) []grantColumn {
	fields := make([]grantColumn, len(names))
	for i, name := range names {
		fields[i] = grantColumn{"t." + name, name}
	}
	return fields
}

func (r grantRelease) appendContainerColumns(fields []grantColumn) []grantColumn {
	if r.common {
		fields = append(fields, grantColumn{"t.COMMON", "COMMON"})
	}
	if r.inherited {
		fields = append(fields, grantColumn{"t.INHERITED", "INHERITED"})
	}
	return fields
}

const granteeSources = "DBA_USERS+DBA_ROLES"

const namedGrantees = ` t JOIN (
	SELECT USERNAME AS PRINCIPAL_NAME, 'USER' AS GRANTEE_KIND FROM DBA_USERS
	UNION ALL
	SELECT ROLE AS PRINCIPAL_NAME, 'ROLE' AS GRANTEE_KIND FROM DBA_ROLES`

const granteeJoinPredicate = `
) p ON NLSSORT(p.PRINCIPAL_NAME, 'NLS_SORT=BINARY') = NLSSORT(t.GRANTEE, 'NLS_SORT=BINARY')
 WHERE `

const notPublicGrantee = `NLSSORT(t.GRANTEE, 'NLS_SORT=BINARY') <> NLSSORT('PUBLIC', 'NLS_SORT=BINARY')`

const joinNamedGrantees = namedGrantees + granteeJoinPredicate + notPublicGrantee

// PUBLIC has its own row kind whether the dictionary lists it as a role or
// omits it. Excluding any named match prevents duplicate PUBLIC grants.
const joinPublicGrantee = namedGrantees + `
	UNION ALL
	SELECT 'PUBLIC' AS PRINCIPAL_NAME, 'PUBLIC' AS GRANTEE_KIND FROM DUAL` +
	granteeJoinPredicate + "(" + notPublicGrantee + " OR p.GRANTEE_KIND = 'PUBLIC')"

const applicationPublicScope = "connected_container_dba_application_public"

func selectClassifiedGrants(options ClassifiedOptions, release grantRelease, source, table, ownerColumn, order string, fields []grantColumn) (selection, error) {
	if options.ApplicationPublicOnly {
		if len(options.Names) != 1 || options.Names[0] != "PUBLIC" {
			return selection{}, ErrInvalidRequest
		}
		if !release.roleMaintained {
			return selection{}, ErrApplicationPublicUnavailable
		}
	}
	expressions := make([]string, len(fields))
	schema := make([]rowset.Column, len(fields))
	for i, field := range fields {
		expressions[i] = field.expression + " AS " + field.alias
		schema[i] = rowset.Column{Name: field.alias, Kind: rowset.Text}
	}
	includePublic := slices.Contains(options.Names, "PUBLIC")
	join := joinNamedGrantees
	if includePublic {
		join = joinPublicGrantee
	}
	statement := "SELECT " + strings.Join(expressions, ", ") + " FROM " + table + join
	scope := "connected_container_dba_dictionary"
	if options.ApplicationPublicOnly {
		scope = applicationPublicScope
		if ownerColumn == "" {
			statement += " AND " + grantedRoleMaintenance + " = 'N'"
		} else {
			statement += ` AND EXISTS (SELECT 1 FROM DBA_USERS u
 WHERE NLSSORT(u.USERNAME, 'NLS_SORT=BINARY') = NLSSORT(t.OWNER, 'NLS_SORT=BINARY')
   AND u.ORACLE_MAINTAINED = 'N')`
		}
	}
	var args []any
	if ownerColumn != "" {
		statement, args = scopeFilter(statement, args, "t."+ownerColumn, options.Owners)
	}
	statement, args = scopeFilter(statement, args, "t.GRANTEE", options.Names)
	return selection{
		ownerColumn:       ownerColumn,
		nameColumn:        "GRANTEE",
		classifiedGrantee: true,
		publicGrantee:     includePublic,
		sql:               statement + " ORDER BY " + order,
		args:              args,
		schema:            schema,
		provenance: Provenance{
			Source:      source,
			OracleMajor: options.OracleMajor,
			Scope:       scope,
			Names:       append([]string(nil), options.Names...),
			Owners:      append([]string(nil), options.Owners...),
		},
	}, nil
}

func optionalGrantValue(row []session.Value, index map[string]int, alias string) *session.Value {
	if position, selected := index[alias]; selected {
		return new(row[position])
	}
	return nil
}

func readClassifiedGrants[T any](
	ctx context.Context, reader Reader, request ClassifiedRequest,
	selectPlan func(ClassifiedOptions) (selection, error),
	decode func(session.Result, selection) (T, error),
) (T, error) {
	var zero T
	plan, err := selectPlan(request.Options)
	if err != nil {
		return zero, err
	}
	result, err := query(ctx, reader, Request{Options: request.Options.Options, Reference: request.Reference, Limits: request.Limits}, plan)
	if err != nil {
		return zero, err
	}
	facts, err := decode(result, plan)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return zero, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}
