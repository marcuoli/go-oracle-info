package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// DatabaseIncarnation preserves exact current/prior RESETLOGS observations.
// Oracle 9.2 projects the four RESETLOGS fields. Later-record identifiers and
// flags remain nil when the release did not project them.
type DatabaseIncarnation struct {
	ResetlogsChange, ResetlogsTime, PriorResetlogsChange, PriorResetlogsTime     session.Value
	Incarnation, PriorIncarnation, ResetlogsID, Status, FlashbackDatabaseAllowed *session.Value
	ContainerID, SourceContainerID                                               *session.Value
}

// DatabaseIncarnationsResult contains the admitted source rows and their provenance.
type DatabaseIncarnationsResult struct {
	Provenance Provenance
	Rows       []DatabaseIncarnation
}

// DatabaseIncarnationsStatement selects documented recovery identity fields.
func DatabaseIncarnationsStatement(version string) (string, error) {
	p, err := databaseIncarnationsPlan(version)
	return p.sql, err
}

func databaseIncarnationsPlan(version string) (selection, error) {
	n, _ := oracleversion.Parse(version)
	if _, ok := oracleversion.LegacyFactMajor(version); !ok {
		return selection{}, ErrUnsupportedVersion
	}
	fields := []field{
		exact("RESETLOGS_CHANGE", "v.RESETLOGS_CHANGE#"), date("RESETLOGS_TIME", "v.RESETLOGS_TIME"),
		exact("PRIOR_RESETLOGS_CHANGE", "v.PRIOR_RESETLOGS_CHANGE#"), date("PRIOR_RESETLOGS_TIME", "v.PRIOR_RESETLOGS_TIME"),
	}
	order := "v.RESETLOGS_TIME, v.RESETLOGS_CHANGE#"
	if n[0] >= 10 {
		fields = append(fields, exact("INCARNATION", "v.INCARNATION#"), exact("PRIOR_INCARNATION", "v.PRIOR_INCARNATION#"),
			exact("RESETLOGS_ID", "v.RESETLOGS_ID"), text("STATUS", "v.STATUS"))
		order = "v.INCARNATION#"
	}
	if n[0] > 10 || (n[0] == 10 && n[1] == 2) {
		fields = append(fields, text("FLASHBACK_DATABASE_ALLOWED", "v.FLASHBACK_DATABASE_ALLOWED"))
	}
	if n[0] >= 12 {
		fields = appendRecoveryContainer(fields)
		order = "v.CON_ID, " + order
	}
	p, err := selectAdmittedView(version, Filter{}, "V$DATABASE_INCARNATION", fields, "", order)
	p.provenance.Scope = "visible_database_incarnations"
	return p, err
}

// DecodeDatabaseIncarnations preserves the exact values and SQL NULLs of every selected row.
func DecodeDatabaseIncarnations(result session.Result, version string) (DatabaseIncarnationsResult, error) {
	p, err := databaseIncarnationsPlan(version)
	if err != nil {
		return DatabaseIncarnationsResult{}, err
	}
	return decodeDatabaseIncarnations(result, p)
}

func decodeDatabaseIncarnations(result session.Result, p selection) (DatabaseIncarnationsResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return DatabaseIncarnationsResult{}, err
	}
	rows := make([]DatabaseIncarnation, len(result.Rows))
	for i, row := range result.Rows {
		v := DatabaseIncarnation{
			ResetlogsChange: row[index["RESETLOGS_CHANGE"]], ResetlogsTime: row[index["RESETLOGS_TIME"]],
			PriorResetlogsChange: row[index["PRIOR_RESETLOGS_CHANGE"]], PriorResetlogsTime: row[index["PRIOR_RESETLOGS_TIME"]],
			Incarnation: recoveryOptional(row, index, "INCARNATION"), PriorIncarnation: recoveryOptional(row, index, "PRIOR_INCARNATION"),
			ResetlogsID: recoveryOptional(row, index, "RESETLOGS_ID"), Status: recoveryOptional(row, index, "STATUS"),
			FlashbackDatabaseAllowed: recoveryOptional(row, index, "FLASHBACK_DATABASE_ALLOWED"),
			ContainerID:              recoveryOptional(row, index, "CON_ID"), SourceContainerID: recoveryOptional(row, index, "SOURCE_CON_ID"),
		}

		rows[i] = v
	}
	return DatabaseIncarnationsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadDatabaseIncarnations completes one bounded observation on an admitted reader.
func ReadDatabaseIncarnations(ctx context.Context, reader Reader, request RecoveryIdentityRequest) (DatabaseIncarnationsResult, error) {
	p, err := databaseIncarnationsPlan(request.OracleVersion)
	if err != nil {
		return DatabaseIncarnationsResult{}, err
	}
	result, err := readRecoveryIdentity(ctx, reader, request, p)
	if err != nil {
		return DatabaseIncarnationsResult{}, err
	}
	facts, err := decodeDatabaseIncarnations(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return DatabaseIncarnationsResult{}, &Error{p.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DatabaseIncarnationsRows transfers the caller-native cursor to its owner.
func DatabaseIncarnationsRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := databaseIncarnationsPlan(version)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// DatabaseIncarnationsJSON reads one bounded complete dataset on a separate cursor.
func DatabaseIncarnationsJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := databaseIncarnationsPlan(version)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits)
}
