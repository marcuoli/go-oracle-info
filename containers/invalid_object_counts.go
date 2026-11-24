package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// InvalidObjectCountsRequest supplies caller admission, a SQL reference, and buffered bounds.
type InvalidObjectCountsRequest struct {
	Options   RootDictionaryOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// InvalidObjectCount is the count of visible INVALID objects for an observed
// container group. Count is exact decimal text and may be zero.
type InvalidObjectCount struct {
	ContainerID session.Value
	Count       session.Value
}

// InvalidObjectCountsResult retains the visible rows and their source provenance.
type InvalidObjectCountsResult struct {
	Provenance Provenance
	Rows       []InvalidObjectCount
}

// InvalidObjectCountsStatement selects the reviewed source without SQL truncation.
func InvalidObjectCountsStatement(options RootDictionaryOptions) (string, error) {
	p, err := invalidObjectCountsPlan(options)
	return p.sql, err
}

func invalidObjectCountsPlan(options RootDictionaryOptions) (selection, error) {
	_, err := rootDictionaryProfile(options)
	if err != nil {
		return selection{}, err
	}
	provenance := Provenance{Source: "CDB_OBJECTS", OracleVersion: options.OracleVersion, Scope: options.Scope}
	return selection{
		sql: "SELECT v.CON_ID AS CON_ID, " +
			"TO_CHAR(COUNT(CASE WHEN v.STATUS = 'INVALID' THEN 1 END), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS INVALID_COUNT " +
			"FROM SYS.CDB_OBJECTS v WHERE " + rootDictionaryPredicate + " GROUP BY v.CON_ID ORDER BY v.CON_ID",
		schema:     []rowset.Column{{Name: "CON_ID", Kind: rowset.Integer}, {Name: "INVALID_COUNT", Kind: rowset.Text}},
		provenance: provenance,
	}, nil
}

// DecodeInvalidObjectCounts validates the supplied complete shape before returning any rows.
func DecodeInvalidObjectCounts(result session.Result, options RootDictionaryOptions) (InvalidObjectCountsResult, error) {
	p, err := invalidObjectCountsPlan(options)
	if err != nil {
		return InvalidObjectCountsResult{}, err
	}
	return decodeInvalidObjectCounts(result, p)
}

func decodeInvalidObjectCounts(result session.Result, p selection) (InvalidObjectCountsResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return InvalidObjectCountsResult{}, err
	}
	seen := make(map[int64]struct{}, len(result.Rows))
	rows := make([]InvalidObjectCount, len(result.Rows))
	for i, row := range result.Rows {
		item := InvalidObjectCount{
			ContainerID: row[index["CON_ID"]],
			Count:       row[index["INVALID_COUNT"]],
		}
		if item.ContainerID.Null || !unsigned(item.ContainerID) {
			return InvalidObjectCountsResult{}, ErrInvalidEvidence
		}
		if item.Count.Null || !unsigned(item.Count) {
			return InvalidObjectCountsResult{}, ErrInvalidEvidence
		}
		if _, duplicate := seen[id(item.ContainerID)]; duplicate {
			return InvalidObjectCountsResult{}, ErrInvalidEvidence
		}
		seen[id(item.ContainerID)] = struct{}{}
		rows[i] = item
	}
	return InvalidObjectCountsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadInvalidObjectCounts executes once over the caller's admitted bounded reader.
func ReadInvalidObjectCounts(ctx context.Context, reader Reader, request InvalidObjectCountsRequest) (InvalidObjectCountsResult, error) {
	p, err := invalidObjectCountsPlan(request.Options)
	if err != nil {
		return InvalidObjectCountsResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return InvalidObjectCountsResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return InvalidObjectCountsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return InvalidObjectCountsResult{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
	}
	facts, decodeErr := decodeInvalidObjectCounts(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return InvalidObjectCountsResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// InvalidObjectCountsRows transfers the actual native cursor to its owner.
func InvalidObjectCountsRows(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions) (*sql.Rows, error) {
	p, err := invalidObjectCountsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, sourceFailure(p.provenance.Source, err, true)
	}
	return rows, nil
}

// InvalidObjectCountsJSON consumes a separate complete bounded cursor.
func InvalidObjectCountsJSON(ctx context.Context, q rowset.Queryer, options RootDictionaryOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := invalidObjectCountsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeInvalidObjectCounts(r, p)
		return err
	})
}
