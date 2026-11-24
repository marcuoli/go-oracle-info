package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
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
	p, err := sQLPatchHistoryPlan(options)
	return p.sql, err
}

func sQLPatchHistoryPlan(options RootDictionaryOptions) (selection, error) {
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
	result := selectFields(p, fields, nil, rootDictionaryPredicate, "v.CON_ID, v.ACTION_TIME, v.PATCH_ID, v.PATCH_UID, v.ACTION")
	return result, nil
}

// DecodeSQLPatchHistory validates the supplied complete shape before returning any rows.
func DecodeSQLPatchHistory(result session.Result, options RootDictionaryOptions) (SQLPatchHistoryResult, error) {
	p, err := sQLPatchHistoryPlan(options)
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
		if !unsigned(item.PatchID) || !unsigned(item.PatchUID) {
			return SQLPatchHistoryResult{}, ErrInvalidEvidence
		}
		if !item.ActionTime.Null {
			const civil = "2006-01-02T15:04:05.000000"
			if _, err := time.Parse(civil, item.ActionTime.Text); err != nil || len(item.ActionTime.Text) != len(civil) || item.ActionTime.Text[19] != '.' {
				return SQLPatchHistoryResult{}, ErrInvalidEvidence
			}
		}
		rows[i] = item
	}
	return SQLPatchHistoryResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadSQLPatchHistory executes once over the caller's admitted bounded reader.
func ReadSQLPatchHistory(ctx context.Context, reader Reader, request SQLPatchHistoryRequest) (SQLPatchHistoryResult, error) {
	p, err := sQLPatchHistoryPlan(request.Options)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return SQLPatchHistoryResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return SQLPatchHistoryResult{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
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
	p, err := sQLPatchHistoryPlan(options)
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
	p, err := sQLPatchHistoryPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeSQLPatchHistory(r, p)
		return err
	})
}
