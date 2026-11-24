package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PDBAccountsOptions identifies one user PDB by its CDB-local container number.
// The caller admits the root session and its dictionary visibility.
type PDBAccountsOptions struct {
	OracleVersion string
	ContainerID   int
}

// PDBAccountsRequest supplies the bounded reader's reference and limits.
type PDBAccountsRequest struct {
	Options   PDBAccountsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// PDBAccount retains raw Oracle account evidence. An empty successful read
// does not establish that the caller can see the requested PDB.
type PDBAccount struct {
	Username         session.Value
	OracleMaintained session.Value
}

func pdbAccountsPlan(options PDBAccountsOptions) (selection, error) {
	version, ok := oracleversion.Parse(options.OracleVersion)
	supported := false
	if ok {
		switch version[0] {
		case 12:
			supported = version[1] == 1 && version[2] == 0 && version[3] >= 2 ||
				version[1] == 2 && version[2] == 0 && version[3] >= 1
		case 18, 19, 21, 23:
			supported = true
		}
	}
	if !supported {
		return selection{}, ErrUnsupportedVersion
	}
	if options.ContainerID <= 2 {
		return selection{}, ErrInvalidRequest
	}
	return selection{
		sql:        "SELECT u.USERNAME AS USERNAME, u.ORACLE_MAINTAINED AS ORACLE_MAINTAINED FROM CDB_USERS u WHERE u.CON_ID = :1 AND SYS_CONTEXT('USERENV','CON_ID') = '1' ORDER BY u.USERNAME",
		args:       []any{options.ContainerID},
		schema:     []rowset.Column{{Name: "USERNAME", Kind: rowset.Text}, {Name: "ORACLE_MAINTAINED", Kind: rowset.Text}},
		provenance: Provenance{Source: "CDB_USERS", OracleMajor: int(version[0]), Scope: "root_dictionary_for_selected_pdb", ContainerID: options.ContainerID},
	}, nil
}

// PDBAccountsStatement returns the selected PDB's bound dictionary projection.
func PDBAccountsStatement(options PDBAccountsOptions) (string, []any, error) {
	p, err := pdbAccountsPlan(options)
	return p.sql, p.args, err
}

// DecodePDBAccounts preserves usernames and maintenance flags including SQL NULL.
func DecodePDBAccounts(result session.Result, options PDBAccountsOptions) ([]PDBAccount, error) {
	p, err := pdbAccountsPlan(options)
	if err != nil {
		return nil, err
	}
	return decodePDBAccounts(result, p)
}

func decodePDBAccounts(result session.Result, p selection) ([]PDBAccount, error) {
	index, err := columns(result, p)
	if err != nil {
		return nil, &Error{Operation: "decode", cause: err}
	}
	records := make([]PDBAccount, len(result.Rows))
	for i, row := range result.Rows {
		records[i] = PDBAccount{Username: row[index["USERNAME"]], OracleMaintained: row[index["ORACLE_MAINTAINED"]]}
	}
	return records, nil
}

// ReadPDBAccounts executes one complete bounded read on the admitted root reader.
func ReadPDBAccounts(ctx context.Context, reader Reader, request PDBAccountsRequest) ([]PDBAccount, error) {
	p, err := pdbAccountsPlan(request.Options)
	if err != nil {
		return nil, err
	}
	result, err := query(ctx, reader, Request{Reference: request.Reference, Limits: request.Limits}, p)
	if err != nil {
		return nil, err
	}
	rows, err := decodePDBAccounts(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{Operation: "read", cause: err}
	}
	return rows, nil
}

// PDBAccountsRows returns an actual caller-owned native cursor.
func PDBAccountsRows(ctx context.Context, q rowset.Queryer, options PDBAccountsOptions) (*sql.Rows, error) {
	p, err := pdbAccountsPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// PDBAccountsJSON returns a complete bounded cursor payload including scope provenance.
func PDBAccountsJSON(ctx context.Context, q rowset.Queryer, options PDBAccountsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := pdbAccountsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits)
}
