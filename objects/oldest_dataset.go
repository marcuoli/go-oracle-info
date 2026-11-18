package objects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// OldestStatisticsRequest retains the existing capability/owner/sample selection
// and adds a caller-owned reference. It never classifies application owners.
type OldestStatisticsRequest struct {
	Options   StatisticsOptions
	Owners    []string
	Reference reference.Reference
}

// OldestStatisticsResult preserves the existing nullable Statistics model.
// Complete refers to the selected sample transport, never the whole dictionary.
type OldestStatisticsResult struct {
	Provenance DatasetProvenance
	Rows       []Statistics
}

func oldestDatasetPlan(options StatisticsOptions, owners []string) (datasetSelection, error) {
	switch options.OracleMajor {
	case 11, 12, 18, 19, 21, 23:
	default:
		return datasetSelection{}, ErrUnsupportedVersion
	}
	query, args, err := OldestStatisticsStatement(options, owners...)
	if err != nil {
		return datasetSelection{}, err
	}
	schema := make([]rowset.Column, 0, 9)
	for _, name := range statisticsNames(options.OracleMajor) {
		schema = append(schema, rowset.Column{Name: name, Kind: rowset.Text})
	}
	return datasetSelection{sql: query, args: args, schema: schema, provenance: DatasetProvenance{Source: "DBA_TAB_STATISTICS", OracleMajor: options.OracleMajor, Scope: "admitted_owner_oldest_statistics_sample", Limit: options.Limit, Owners: append([]string(nil), owners...), OracleMaintainedFilter: options.OracleMaintainedAvailable}}, nil
}

// DecodeOldestStatistics adds source/sample provenance around the existing pure
// decoder, retaining nullable fields and explicit optional SCOPE admission.
func DecodeOldestStatistics(result session.Result, options StatisticsOptions, owners ...string) (OldestStatisticsResult, error) {
	p, err := oldestDatasetPlan(options, owners)
	if err != nil {
		return OldestStatisticsResult{}, err
	}
	return decodeOldestDataset(result, p)
}
func decodeOldestDataset(result session.Result, p datasetSelection) (OldestStatisticsResult, error) {
	if _, err := datasetColumns(result, p); err != nil {
		return OldestStatisticsResult{}, err
	}
	rows, err := DecodeStatistics(result, p.provenance.OracleMajor, p.provenance.Limit)
	if err != nil {
		return OldestStatisticsResult{}, errors.Join(ErrInvalidEvidence, err)
	}
	for _, r := range rows {
		if len(p.provenance.Owners) > 0 {
			admitted := false
			for _, owner := range p.provenance.Owners {
				admitted = admitted || (r.Owner.Valid && r.Owner.String == owner)
			}
			if !admitted {
				return OldestStatisticsResult{}, ErrInvalidEvidence
			}
		}
	}
	return OldestStatisticsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadOldestStatistics executes the admitted sample once and publishes no failed
// prefix. Source limits and Oracle-maintained capability remain explicit.
func ReadOldestStatistics(ctx context.Context, reader Reader, request OldestStatisticsRequest) (OldestStatisticsResult, error) {
	p, err := oldestDatasetPlan(request.Options, request.Owners)
	if err != nil {
		return OldestStatisticsResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return OldestStatisticsResult{}, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return OldestStatisticsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = datasetReadError(ctx, p, err); err != nil {
		return OldestStatisticsResult{}, err
	}
	facts, err := decodeOldestDataset(result, p)
	if err = datasetReadError(ctx, p, err); err != nil {
		return OldestStatisticsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// OldestStatisticsRows returns the actual native sample cursor. Caller owns its
// complete lifecycle/budgets; a full sample never proves dictionary completeness.
func OldestStatisticsRows(ctx context.Context, q rowset.Queryer, options StatisticsOptions, owners ...string) (*sql.Rows, error) {
	p, err := oldestDatasetPlan(options, owners)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// OldestStatisticsJSON owns a separate cursor, preserves source order/NULLs and
// publishes no bytes on terminal, close, cancellation, domain or capacity errors.
func OldestStatisticsJSON(ctx context.Context, q rowset.Queryer, options StatisticsOptions, limits rowset.Limits, owners ...string) (json.RawMessage, error) {
	p, err := oldestDatasetPlan(options, owners)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := decodeOldestDataset(r, p); return err })
}
