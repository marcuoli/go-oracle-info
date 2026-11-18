package goldengate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Administrator is a privilege observation, not proof of a running process,
// active login, object ownership or configured GGSCHEMA. Values remain exact.
type Administrator struct {
	Username              session.Value
	PrivilegeType         session.Value
	GrantSelectPrivileges session.Value
}

// AdministratorsResult preserves every selected row in the admitted dictionary
// scope. Complete describes transport, not all PDBs or GoldenGate deployments.
type AdministratorsResult struct {
	Provenance Provenance
	Rows       []Administrator
}

// AdministratorsStatement selects documented privilege metadata without probing
// or falling back to a narrower USER/ALL view after a denied database-wide read.
func AdministratorsStatement(version string) (string, error) {
	p, e := administratorsPlan(version)
	return p.sql, e
}
func administratorsPlan(version string) (selection, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 5 {
		return selection{}, ErrUnsupportedVersion
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return selection{}, ErrUnsupportedVersion
		}
		if _, e := strconv.ParseUint(part, 10, 32); e != nil {
			return selection{}, ErrUnsupportedVersion
		}
	}
	admitted := false
	switch parts[0] {
	case "12":
		admitted = parts[1] == "2"
	case "18", "19", "21", "23":
		admitted = true
	}
	if !admitted {
		return selection{}, ErrUnsupportedVersion
	}
	return selection{sql: "SELECT v.USERNAME AS USERNAME, v.PRIVILEGE_TYPE AS PRIVILEGE_TYPE, v.GRANT_SELECT_PRIVILEGES AS GRANT_SELECT_PRIVILEGES FROM DBA_GOLDENGATE_PRIVILEGES v ORDER BY v.USERNAME, v.PRIVILEGE_TYPE, v.GRANT_SELECT_PRIVILEGES", schema: []rowset.Column{{Name: "USERNAME", Kind: rowset.Text}, {Name: "PRIVILEGE_TYPE", Kind: rowset.Text}, {Name: "GRANT_SELECT_PRIVILEGES", Kind: rowset.Text}}, provenance: Provenance{Source: "DBA_GOLDENGATE_PRIVILEGES", Scope: "admitted_container_dictionary", OracleVersion: version}}, nil
}

// DecodeAdministrators validates the entire projection. It does not establish
// completion, current process use or whether GoldenGate is installed/running.
func DecodeAdministrators(result session.Result, version string) (AdministratorsResult, error) {
	p, e := administratorsPlan(version)
	if e != nil {
		return AdministratorsResult{}, e
	}
	index, e := columns(result, p)
	if e != nil {
		return AdministratorsResult{}, e
	}
	rows := make([]Administrator, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Administrator{Username: row[index["USERNAME"]], PrivilegeType: row[index["PRIVILEGE_TYPE"]], GrantSelectPrivileges: row[index["GRANT_SELECT_PRIVILEGES"]]}
	}
	return AdministratorsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadAdministrators executes once through the admitted complete-result reader.
// Late failures and post-query cancellation publish no successful row prefix.
func ReadAdministrators(ctx context.Context, reader Reader, request Request) (AdministratorsResult, error) {
	p, e := administratorsPlan(request.OracleVersion)
	if e != nil {
		return AdministratorsResult{}, e
	}
	result, e := query(ctx, reader, request, p)
	if e != nil {
		return AdministratorsResult{Provenance: p.provenance}, e
	}
	facts, e := DecodeAdministrators(result, request.OracleVersion)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return AdministratorsResult{Provenance: p.provenance}, e
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// AdministratorsRows returns the actual caller-native cursor; callers retain
// Next/Scan/Err/Close, handle affinity, deadlines and finite stream budgets.
func AdministratorsRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, e := administratorsPlan(version)
	if e != nil {
		return nil, e
	}
	rows, e := rowset.Query(ctx, q, p.sql)
	if e != nil {
		return nil, &Error{p.provenance.Source, e}
	}
	return rows, nil
}

// AdministratorsJSON independently consumes its cursor with complete domain
// admission. No bytes survive query, scan, close, cancellation or capacity failure.
func AdministratorsJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, e := administratorsPlan(version)
	if e != nil {
		return nil, e
	}
	return nativeJSON(ctx, q, p, limits)
}
