package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// DatabaseFile retains a visible controlfile data/temp file, without RAC
// duplication. ContainerID is absent on 11g, distinct from a selected SQL NULL.
type DatabaseFile struct {
	Type, FileID, Name, Bytes session.Value
	ContainerID               *session.Value
}

// DatabaseFilesResult owns complete selected rows, not a claim that privileges
// expose every physical file or container. Source facts remain unrounded bytes.
type DatabaseFilesResult struct {
	Provenance Provenance
	Rows       []DatabaseFile
}

// DatabaseFilesStatement reads controlfile metadata without opening files or
// changing containers. Root sees admitted visible containers; a PDB stays scoped
// by Oracle. Callers admit view grants and mounted/open-state source availability.
func DatabaseFilesStatement(major int) (string, error) {
	plan, err := databaseFilesPlan(major)
	return plan.sql, err
}

func databaseFilesPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{{Name: "FILE_TYPE", Kind: rowset.Text}, {Name: "FILE_ID", Kind: rowset.Integer}, {Name: "FILE_NAME", Kind: rowset.Text}, {Name: "BYTES", Kind: rowset.Integer}}
	container := ""
	order := "FILE_TYPE,FILE_ID"
	if major >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		container = ", CON_ID AS CON_ID"
		order = "FILE_TYPE,CON_ID,FILE_ID"
	}
	sql := "SELECT 'DATAFILE' AS FILE_TYPE, FILE# AS FILE_ID, NAME AS FILE_NAME, BYTES AS BYTES" + container + " FROM V$DATAFILE UNION ALL SELECT 'TEMPFILE' AS FILE_TYPE, FILE# AS FILE_ID, NAME AS FILE_NAME, BYTES AS BYTES" + container + " FROM V$TEMPFILE ORDER BY " + order
	return selection{sql: sql, schema: schema, provenance: Provenance{Source: "V$DATAFILE,V$TEMPFILE", Scope: "visible_controlfile", OracleMajor: major}}, nil
}

// DecodeDatabaseFiles validates every row before publishing it. A pure decode
// does not establish transport completion; ReadDatabaseFiles sets Complete.
func DecodeDatabaseFiles(result session.Result, major int) (DatabaseFilesResult, error) {
	plan, err := databaseFilesPlan(major)
	if err != nil {
		return DatabaseFilesResult{}, err
	}
	return decodeDatabaseFiles(result, plan)
}

func decodeDatabaseFiles(result session.Result, plan selection) (DatabaseFilesResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return DatabaseFilesResult{}, err
	}
	rows := make([]DatabaseFile, 0, len(result.Rows))
	seen := map[string]bool{}
	for _, row := range result.Rows {
		file := DatabaseFile{Type: row[index["FILE_TYPE"]], FileID: row[index["FILE_ID"]], Name: row[index["FILE_NAME"]], Bytes: row[index["BYTES"]]}
		if file.Type.Null || (file.Type.Text != "DATAFILE" && file.Type.Text != "TEMPFILE") {
			return DatabaseFilesResult{}, ErrInvalidEvidence
		}
		id, err := strconv.ParseInt(file.FileID.Text, 10, 64)
		if file.FileID.Null || err != nil || id <= 0 {
			return DatabaseFilesResult{}, ErrInvalidEvidence
		}
		container := "unselected"
		if pos, ok := index["CON_ID"]; ok {
			v := row[pos]
			file.ContainerID = &v
			if v.Null {
				container = "null"
			} else {
				numeric, _ := new(big.Int).SetString(v.Text, 10)
				container = numeric.String()
			}
		}
		// Numeric identities may have alternate textual encodings. Canonicalize
		// only the lookup key so a duplicate cannot inflate allocation; raw fields
		// remain unchanged for provenance and native/JSON consumers.
		key := file.Type.Text + ":" + strconv.FormatInt(id, 10) + ":" + container
		if seen[key] {
			return DatabaseFilesResult{}, ErrInvalidEvidence
		}
		seen[key] = true
		rows = append(rows, file)
	}
	return DatabaseFilesResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadDatabaseFiles executes once on a caller-owned admitted reader. Any query,
// decode or post-query cancellation failure returns no row prefix.
func ReadDatabaseFiles(ctx context.Context, reader Reader, request Request) (DatabaseFilesResult, error) {
	plan, err := databaseFilesPlan(request.OracleMajor)
	if err != nil {
		return DatabaseFilesResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return DatabaseFilesResult{}, err
	}
	facts, decodeErr := decodeDatabaseFiles(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return DatabaseFilesResult{}, &Error{plan.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatabaseFilesRows returns the actual caller-owned native cursor. Its caller
// owns Next/Scan/Err/Close, admission, deadlines and finite stream budgets.
func DatabaseFilesRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := databaseFilesPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// DatabaseFilesJSON independently consumes the same source through shared
// rowset limits and terminal/close handling. No failed prefix is returned.
func DatabaseFilesJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := databaseFilesPlan(major)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(r session.Result) error { _, err := decodeDatabaseFiles(r, plan); return err })
}

// DatafileBytes sums exact visible DATAFILE allocation, excluding tempfiles,
// redo, control files and free/used-space estimates. Empty/incomplete sources,
// NULL sizes and zero (documented inaccessible V$DATAFILE.BYTES) remain unknown.
func (r DatabaseFilesResult) DatafileBytes() session.Value {
	unknown := session.Value{Null: true}
	if !r.Provenance.Complete {
		return unknown
	}
	sum := new(big.Int)
	count := 0
	for _, file := range r.Rows {
		if file.Type.Text != "DATAFILE" {
			continue
		}
		count++
		n, ok := new(big.Int).SetString(file.Bytes.Text, 10)
		if file.Bytes.Null || !ok || n.Sign() <= 0 {
			return unknown
		}
		sum.Add(sum, n)
	}
	if count == 0 {
		return unknown
	}
	return session.Value{Text: sum.String()}
}

// StorageType classifies all visible data/temp paths as ASM, FileSystem or
// ASM+FileSystem. Empty/incomplete inventories, missing names or unsupported
// Exascale vault paths remain unknown;
// this property does not identify Exadata or any hardware family.
func (r DatabaseFilesResult) StorageType() session.Value {
	unknown := session.Value{Null: true}
	if !r.Provenance.Complete || len(r.Rows) == 0 {
		return unknown
	}
	asm, filesystem := false, false
	for _, file := range r.Rows {
		if file.Name.Null || file.Name.Text == "" {
			return unknown
		}
		// Oracle Exascale vault paths are not filesystems despite their directory
		// syntax. Preserve raw paths/allocation and keep this three-value property
		// unknown rather than mislabeling an unsupported storage backend.
		if strings.HasPrefix(file.Name.Text, "@") {
			return unknown
		}
		if strings.HasPrefix(file.Name.Text, "+") {
			asm = true
		} else {
			filesystem = true
		}
	}
	text := "FileSystem"
	if asm {
		text = "ASM"
		if filesystem {
			text = "ASM+FileSystem"
		}
	}
	return session.Value{Text: text}
}
