package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// SPFileRequest keeps stored SID clauses separate from reporting instances.
type SPFileRequest struct {
	OracleMajor int
	Filter      SPFileFilter
	Reference   reference.Reference
}

// SPFileParameter is a raw stored clause. Type is text, not a numeric enum, and
// SID applicability is independent of InstanceID. No precedence is resolved.
type SPFileParameter struct {
	InstanceID    session.Value
	ContainerID   *session.Value
	SID           session.Value
	Name          session.Value
	Type          session.Value
	Ordinal       session.Value
	Value         session.Value
	DisplayValue  session.Value
	IsSpecified   session.Value
	UpdateComment session.Value
}

// SPFileResult retains complete stored clauses, including wildcard/dormant SIDs.
type SPFileResult struct {
	Provenance Provenance
	Rows       []SPFileParameter
}

// DecodeSPFile validates stored clauses without applying effective-value policy.
func DecodeSPFile(result session.Result, major int, filter SPFileFilter) (SPFileResult, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return SPFileResult{}, err
	}
	return decodeSPFile(result, plan)
}

func decodeSPFile(result session.Result, plan selection) (SPFileResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return SPFileResult{}, err
	}
	rows := make([]SPFileParameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = SPFileParameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			SID: row[index["SID"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Ordinal: row[index["ORDINAL"]], Value: row[index["VALUE"]],
			DisplayValue: row[index["DISPLAY_VALUE"]], IsSpecified: row[index["ISSPECIFIED"]],
			UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return SPFileResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadSPFile preserves the reader's original failure and never decodes a prefix.
func ReadSPFile(ctx context.Context, reader Reader, request SPFileRequest) (SPFileResult, error) {
	plan, err := spfilePlan(request.OracleMajor, request.Filter)
	if err != nil {
		return SPFileResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return SPFileResult{}, err
	}
	facts, decodeErr := decodeSPFile(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return SPFileResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
}

// SPFileRows preserves all selected stored clauses, wildcard/defaults and raw
// ISSPECIFIED. Its native cursor has the same ownership as EffectiveRows.
func SPFileRows(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter) (*sql.Rows, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// SPFileJSON is the independent JSON counterpart of SPFileRows. No value
// precedence, SPFILE-use or physical-file-completeness inference is performed.
func SPFileJSON(ctx context.Context, q rowset.Queryer, major int, filter SPFileFilter, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := spfilePlan(major, filter)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits)
}

// SPFileOptions carries the source selectors for the options-based fact APIs.
type SPFileOptions struct {
	OracleMajor int
	Filter      SPFileFilter
}

// SPFileStatementWithOptions selects the same source as SPFileStatement using one options value.
func SPFileStatementWithOptions(options SPFileOptions) (string, []any, error) {
	return SPFileStatement(options.OracleMajor, options.Filter)
}

// DecodeSPFileWithOptions selects the same source as DecodeSPFile using one options value.
func DecodeSPFileWithOptions(result session.Result, options SPFileOptions) (SPFileResult, error) {
	return DecodeSPFile(result, options.OracleMajor, options.Filter)
}

// SPFileRowsWithOptions selects the same source as SPFileRows using one options value.
func SPFileRowsWithOptions(ctx context.Context, q rowset.Queryer, options SPFileOptions) (*sql.Rows, error) {
	return SPFileRows(ctx, q, options.OracleMajor, options.Filter)
}

// SPFileJSONWithOptions selects the same source as SPFileJSON using one options value.
func SPFileJSONWithOptions(ctx context.Context, q rowset.Queryer, options SPFileOptions, limits rowset.Limits) (json.RawMessage, error) {
	return SPFileJSON(ctx, q, options.OracleMajor, options.Filter, limits)
}

// SPFileFilter distinguishes reporting INST_ID from stored SID applicability.
// Empty SIDs preserves all clauses, including dormant SIDs. Named SIDs also
// retain wildcard defaults; rows are never merged into inferred effective values.
type SPFileFilter struct {
	Names     []string
	Instances []int64
	SIDs      []string
}

// SPFileStatement retains raw stored clauses and ISSPECIFIED, even when FALSE.
// INST_ID is the reporting instance; it is never equated to SID applicability.
func SPFileStatement(major int, filter SPFileFilter) (string, []any, error) {
	plan, err := spfilePlan(major, filter)
	return plan.sql, plan.args, err
}

func spfilePlan(major int, filter SPFileFilter) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	sids := append([]string(nil), filter.SIDs...)
	if len(sids) > 0 {
		found := false
		for _, sid := range sids {
			found = found || sid == "*"
		}
		if !found {
			sids = append(sids, "*")
		}
	}
	if err := validate(filter.Names, filter.Instances, sids); err != nil {
		return selection{}, err
	}
	provenance := source("GV$SPPARAMETER", major, filter.Names, filter.Instances)
	provenance.SIDs = sids
	provenance.SIDPolicy = "all_stored_clauses"
	if len(sids) > 0 {
		provenance.SIDPolicy = "selected_sids_plus_wildcard"
	}
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	for _, name := range []string{"SID", "NAME", "TYPE", "ORDINAL", "VALUE", "DISPLAY_VALUE", "ISSPECIFIED", "UPDATE_COMMENT"} {
		kind := rowset.Text
		if name == "ORDINAL" {
			kind = rowset.Integer
		}
		columns = append(columns, rowset.Column{Name: name, Kind: kind})
	}
	return build(major, provenance, columns, sids), nil
}
