package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SQLPatchHistoryRequest supplies caller admission, a SQL reference, and buffered bounds.
type SQLPatchHistoryRequest struct {
	Options   RootDictionaryOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// SQLPatchAttempt preserves one visible datapatch attempt. ActionTime is
// Gregorian civil text with six fractional digits and no timezone.
type SQLPatchAttempt struct {
	ContainerID session.Value
	PatchID     session.Value
	PatchUID    session.Value
	Action      session.Value
	Status      session.Value
	ActionTime  session.Value
	Description session.Value
}

// SQLPatchHistoryResult retains the visible rows and their source provenance.
type SQLPatchHistoryResult struct {
	Provenance Provenance
	Rows       []SQLPatchAttempt
}

// SQLPatchHistoryStatement selects the reviewed source without SQL truncation.
func SQLPatchHistoryStatement(options RootDictionaryOptions) (string, error) {
	p, err := sqlPatchHistoryPlan(options)
	return p.sql, err
}

func sqlPatchHistoryPlan(options RootDictionaryOptions) (selection, error) {
	return rootPatchPlan(options, false)
}

func rootPatchPlan(options RootDictionaryOptions, withRelease bool) (selection, error) {
	r, err := rootDictionaryProfile(options)
	if err != nil {
		return selection{}, err
	}
	if r < release1212 {
		return selection{}, ErrUnsupportedVersion
	}
	p := Provenance{Source: "CDB_REGISTRY_SQLPATCH", OracleVersion: options.OracleVersion, Scope: options.Scope}
	fields := []field{
		raw("CON_ID", rowset.Integer),
		exact("PATCH_ID"),
		exact("PATCH_UID"),
		raw("ACTION", rowset.Text),
		raw("STATUS", rowset.Text),
		projected("ACTION_TIME", datetime.GregorianTimestamp6("v.ACTION_TIME"), rowset.Text),
		raw("DESCRIPTION", rowset.Text),
	}
	releaseColumns := ""
	major, _ := oracleversion.LegacyFactMajor(options.OracleVersion)
	if withRelease && major >= 18 {
		p.OptionalColumns = []string{"PATCH_TYPE", "TARGET_VERSION", "FLAGS"}
		for _, name := range p.OptionalColumns {
			fields = append(fields, raw(name, rowset.Text))
			releaseColumns += ", v." + name
		}
	}
	// A sort in the formatted dictionary query block can raise ORA-12850 on
	// RAC. Keep row formatting in the non-merged view and sort the result
	// outside it. The transported patch IDs are integer text.
	result := selectFields("SYS."+p.Source, p, fields, nil, rootDictionaryPredicate, "")
	result.sql = "SELECT /*+ NO_MERGE(v) */ v.CON_ID, v.PATCH_ID, v.PATCH_UID, v.ACTION, v.STATUS, v.ACTION_TIME, v.DESCRIPTION" + releaseColumns + " FROM (" +
		result.sql + ") v ORDER BY v.CON_ID, v.ACTION_TIME, TO_NUMBER(v.PATCH_ID), TO_NUMBER(v.PATCH_UID), v.ACTION"
	return result, nil
}

// DecodeSQLPatchHistory validates the supplied complete shape before returning any rows.
func DecodeSQLPatchHistory(result session.Result, options RootDictionaryOptions) (SQLPatchHistoryResult, error) {
	p, err := sqlPatchHistoryPlan(options)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	return decodeSQLPatchHistory(result, p)
}

func decodeSQLPatchHistory(result session.Result, p selection) (SQLPatchHistoryResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	return decodeRootPatchRows(result, p, index)
}

func decodeRootPatchRows(result session.Result, p selection, index map[string]int) (SQLPatchHistoryResult, error) {
	major, _ := oracleversion.LegacyFactMajor(p.provenance.OracleVersion)
	rows := make([]SQLPatchAttempt, len(result.Rows))
	for i, row := range result.Rows {
		item := SQLPatchAttempt{
			ContainerID: row[index["CON_ID"]],
			PatchID:     row[index["PATCH_ID"]],
			PatchUID:    row[index["PATCH_UID"]],
			Action:      row[index["ACTION"]],
			Status:      row[index["STATUS"]],
			ActionTime:  row[index["ACTION_TIME"]],
			Description: row[index["DESCRIPTION"]],
		}
		if item.ContainerID.Null || !unsigned(item.ContainerID) {
			return SQLPatchHistoryResult{}, ErrInvalidEvidence
		}
		if item.PatchID.Null || item.PatchUID.Null || !unsigned(item.PatchID) || !unsigned(item.PatchUID) ||
			!requiredText(item.Action) || (major >= 18 && !requiredText(item.Status)) {
			return SQLPatchHistoryResult{}, ErrInvalidEvidence
		}
		const civil = "2006-01-02T15:04:05.000000"
		if _, err := time.Parse(civil, item.ActionTime.Text); item.ActionTime.Null || err != nil ||
			len(item.ActionTime.Text) != len(civil) || item.ActionTime.Text[19] != '.' {
			return SQLPatchHistoryResult{}, ErrInvalidEvidence
		}
		rows[i] = item
	}
	return SQLPatchHistoryResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSQLPatchHistory executes once over the caller's admitted bounded reader.
func ReadSQLPatchHistory(ctx context.Context, reader Reader, request SQLPatchHistoryRequest) (SQLPatchHistoryResult, error) {
	p, err := sqlPatchHistoryPlan(request.Options)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	result, err := readRootPatchRows(ctx, reader, request, p)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	facts, decodeErr := decodeSQLPatchHistory(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return SQLPatchHistoryResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SQLPatchHistoryRows transfers the actual native cursor to its owner.
func SQLPatchHistoryRows(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions) (*sql.Rows, error) {
	p, err := sqlPatchHistoryPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, sourceFailure(p.provenance.Source, err, true)
	}
	return rows, nil
}

// SQLPatchHistoryJSON consumes a separate complete bounded cursor.
func SQLPatchHistoryJSON(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := sqlPatchHistoryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeSQLPatchHistory(r, p)
		return err
	})
}

func readRootPatchRows(ctx context.Context, reader Reader, request SQLPatchHistoryRequest, p selection) (session.Result, error) {
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return session.Result{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return session.Result{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return session.Result{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
	}
	return result, nil
}

// SQLPatchReleaseAttempt retains the base attempt and its release fields.
// A nil pointer denotes an unselected column. A selected SQL NULL stays NULL.
type SQLPatchReleaseAttempt struct {
	SQLPatchAttempt
	PatchType     *session.Value
	TargetVersion *session.Value
	Flags         *session.Value
}

// SQLPatchReleaseResult describes the visible release-aware root history.
type SQLPatchReleaseResult struct {
	Provenance Provenance
	Rows       []SQLPatchReleaseAttempt
}

// SQLPatchReleaseStatement selects root DATAPATCH history with explicit release
// fields where available according to Oracle's dictionary contract.
func SQLPatchReleaseStatement(options RootDictionaryOptions) (string, error) {
	p, err := rootPatchPlan(options, true)
	return p.sql, err
}

// DecodeSQLPatchRelease validates the full version-selected row shape.
// Callers separately verify the completion and origin of persisted evidence.
func DecodeSQLPatchRelease(result session.Result, options RootDictionaryOptions) (SQLPatchReleaseResult, error) {
	p, err := rootPatchPlan(options, true)
	if err != nil {
		return SQLPatchReleaseResult{}, err
	}
	return decodeSQLPatchRelease(result, p)
}

func decodeSQLPatchRelease(result session.Result, p selection) (SQLPatchReleaseResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return SQLPatchReleaseResult{}, err
	}
	history, err := decodeRootPatchRows(result, p, index)
	if err != nil {
		return SQLPatchReleaseResult{}, err
	}
	rows := make([]SQLPatchReleaseAttempt, len(history.Rows))
	for i, attempt := range history.Rows {
		raw := result.Rows[i]
		rows[i] = SQLPatchReleaseAttempt{SQLPatchAttempt: attempt,
			PatchType: optional(raw, index, "PATCH_TYPE"), TargetVersion: optional(raw, index, "TARGET_VERSION"), Flags: optional(raw, index, "FLAGS")}
	}
	return SQLPatchReleaseResult{Provenance: history.Provenance, Rows: rows}, nil
}

// ReadSQLPatchRelease reads the release-aware root inventory in one bounded call.
func ReadSQLPatchRelease(ctx context.Context, reader Reader, request SQLPatchHistoryRequest) (SQLPatchReleaseResult, error) {
	p, err := rootPatchPlan(request.Options, true)
	if err != nil {
		return SQLPatchReleaseResult{}, err
	}
	result, err := readRootPatchRows(ctx, reader, request, p)
	if err != nil {
		return SQLPatchReleaseResult{}, err
	}
	facts, decodeErr := decodeSQLPatchRelease(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return SQLPatchReleaseResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// SQLPatchReleaseRows transfers the actual release-aware native cursor to its owner.
func SQLPatchReleaseRows(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions) (*sql.Rows, error) {
	p, err := rootPatchPlan(options, true)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, sourceFailure(p.provenance.Source, err, true)
	}
	return rows, nil
}

// SQLPatchReleaseJSON consumes a separate complete bounded release-history cursor.
func SQLPatchReleaseJSON(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := rootPatchPlan(options, true)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeSQLPatchRelease(r, p)
		return err
	})
}
