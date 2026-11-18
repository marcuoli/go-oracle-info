package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// DatabaseContext preserves source values and SQL NULL without derived health or topology.
// Exact SCNs are decimal text; native integer columns retain numeric JSON kinds.
type DatabaseContext struct {
	DBID            session.Value
	Name            session.Value
	DBUniqueName    session.Value
	DatabaseRole    session.Value
	OpenMode        session.Value
	ResetlogsChange session.Value
	ResetlogsTime   session.Value
	CurrentSCN      session.Value
	CDB             session.Value
	ContainerID     session.Value
}

// DatabaseContextResult contains all admitted rows; Decode alone cannot prove transport completion.
type DatabaseContextResult struct {
	Provenance Provenance
	Rows       []DatabaseContext
}

// DatabaseContextStatement selects a reviewed source and bound reporter filters, without a row cap.
func DatabaseContextStatement(version string) (string, []any, error) {
	p, e := databaseContextPlan(version)
	return p.sql, p.args, e
}
func databaseContextPlan(version string) (selection, error) {
	p, err := selectView(version, Filter{}, "V$DATABASE", []field{
		exact("DBID", "v.DBID"),
		text("NAME", "v.NAME"),
		text("DB_UNIQUE_NAME", "v.DB_UNIQUE_NAME"),
		text("DATABASE_ROLE", "v.DATABASE_ROLE"),
		text("OPEN_MODE", "v.OPEN_MODE"),
		exact("RESETLOGS_CHANGE", "v.RESETLOGS_CHANGE#"),
		date("RESETLOGS_TIME", "v.RESETLOGS_TIME"),
		exact("CURRENT_SCN", "v.CURRENT_SCN"),
		text("CDB", "v.CDB"),
		integer("CON_ID", "v.CON_ID"),
	}, "", "")
	p.single = true
	p.provenance.Scope = "connected_database"
	return p, err

}

// DecodeDatabaseContext validates the entire projection before publishing any typed rows.
func DecodeDatabaseContext(result session.Result, version string) (DatabaseContextResult, error) {
	p, e := databaseContextPlan(version)
	if e != nil {
		return DatabaseContextResult{}, e
	}
	return decodeDatabaseContext(result, p)
}
func decodeDatabaseContext(result session.Result, p selection) (DatabaseContextResult, error) {
	index, e := columns(result, p)
	if e != nil {
		return DatabaseContextResult{}, e
	}
	rows := make([]DatabaseContext, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = DatabaseContext{
			DBID:            row[index["DBID"]],
			Name:            row[index["NAME"]],
			DBUniqueName:    row[index["DB_UNIQUE_NAME"]],
			DatabaseRole:    row[index["DATABASE_ROLE"]],
			OpenMode:        row[index["OPEN_MODE"]],
			ResetlogsChange: row[index["RESETLOGS_CHANGE"]],
			ResetlogsTime:   row[index["RESETLOGS_TIME"]],
			CurrentSCN:      row[index["CURRENT_SCN"]],
			CDB:             row[index["CDB"]],
			ContainerID:     row[index["CON_ID"]],
		}
	}
	return DatabaseContextResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadDatabaseContext executes once over a caller-admitted complete-result reader.
// Query, terminal, decode or post-query cancellation failure publishes no prefix.
func ReadDatabaseContext(ctx context.Context, reader Reader, request DatabaseContextRequest) (DatabaseContextResult, error) {
	p, e := databaseContextPlan(request.OracleVersion)
	if e != nil {
		return DatabaseContextResult{}, e
	}
	result, e := query(ctx, reader, request.Reference, p)
	if e != nil {
		return DatabaseContextResult{}, e
	}
	facts, e := decodeDatabaseContext(result, p)
	if e = errors.Join(e, ctx.Err()); e != nil {
		return DatabaseContextResult{}, &Error{p.provenance.Source, e}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatabaseContextRows returns the actual caller-native cursor; callers own Err/Close and budgets.
func DatabaseContextRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, e := databaseContextPlan(version)
	if e != nil {
		return nil, e
	}
	return open(ctx, q, p)
}

// DatabaseContextJSON owns its separate cursor and returns nothing on any incomplete read.
func DatabaseContextJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, e := databaseContextPlan(version)
	if e != nil {
		return nil, e
	}
	return readJSON(ctx, q, p, limits)
}
