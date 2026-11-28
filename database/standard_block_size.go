package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// StandardBlockSizeSource independently selects the parameter or the connected
// dictionary's SYSTEM tablespace. Other tablespaces may use nonstandard sizes.
type StandardBlockSizeSource string

const (
	StandardBlockSizeParameter        StandardBlockSizeSource = "V$PARAMETER"
	StandardBlockSizeSystemTablespace StandardBlockSizeSource = "DBA_TABLESPACES(SYSTEM)"
)

// StandardBlockSizeOptions admits an observed numeric release. The zero source
// selects V$PARAMETER. Selecting the SYSTEM source requires an admitted open
// dictionary in the current container.
type StandardBlockSizeOptions struct {
	OracleVersion string
	Source        StandardBlockSizeSource
}

// StandardBlockSizeRequest supplies limits and a reference for an admitted
// handle. Enable the SYSTEM fallback only when its dictionary scope is admitted.
type StandardBlockSizeRequest struct {
	Options                       StandardBlockSizeOptions
	Reference                     reference.Reference
	Limits                        rowset.Limits
	AllowSystemTablespaceFallback bool
}

// StandardBlockSizeRow preserves exact byte text or SQL NULL.
type StandardBlockSizeRow struct{ Bytes session.Value }

// StandardBlockSizeAttempt records a sanitized source and outcome of a read.
type StandardBlockSizeAttempt struct {
	Source  StandardBlockSizeSource
	Outcome string
}

// StandardBlockSizeResult contains at most one row and its actual source.
// Attempts distinguish a returned NULL, an empty source and a failed read.
type StandardBlockSizeResult struct {
	Provenance DatasetProvenance
	Rows       []StandardBlockSizeRow
	Attempts   []StandardBlockSizeAttempt
}

// StandardBlockSizeStatement selects exactly one reviewed source.
func StandardBlockSizeStatement(options StandardBlockSizeOptions) (string, error) {
	plan, err := standardBlockSizePlan(options)
	return plan.sql, err
}

func standardBlockSizePlan(options StandardBlockSizeOptions) (datasetSelection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return datasetSelection{}, ErrUnsupportedVersion
	}
	plan := datasetSelection{
		schema:     []rowset.Column{{Name: "BLOCK_SIZE_BYTES", Kind: rowset.Integer}},
		provenance: DatasetProvenance{OracleMajor: major},
	}
	switch options.Source {
	case "", StandardBlockSizeParameter:
		plan.sql = "SELECT p.VALUE AS BLOCK_SIZE_BYTES FROM V$PARAMETER p WHERE p.NAME = 'db_block_size'"
		plan.provenance.Source = string(StandardBlockSizeParameter)
		plan.provenance.Scope = "connected_database_standard_block_size"
	case StandardBlockSizeSystemTablespace:
		// Oracle uses the standard database block size for SYSTEM, even
		// when other tablespaces in this dictionary have nonstandard sizes.
		plan.sql = "SELECT t.BLOCK_SIZE AS BLOCK_SIZE_BYTES FROM DBA_TABLESPACES t WHERE t.TABLESPACE_NAME = 'SYSTEM'"
		plan.provenance.Source = string(StandardBlockSizeSystemTablespace)
		plan.provenance.Scope = "connected_dictionary_system_tablespace"
	default:
		return datasetSelection{}, ErrInvalidRequest
	}
	return plan, nil
}

// DecodeStandardBlockSize validates the complete shape and exact positive bytes.
func DecodeStandardBlockSize(result session.Result, options StandardBlockSizeOptions) (StandardBlockSizeResult, error) {
	plan, err := standardBlockSizePlan(options)
	if err != nil {
		return StandardBlockSizeResult{}, err
	}
	return decodeStandardBlockSize(result, plan)
}

func decodeStandardBlockSize(result session.Result, plan datasetSelection) (StandardBlockSizeResult, error) {
	index, err := datasetColumns(result, plan, "BLOCK_SIZE_BYTES")
	if err != nil || len(result.Rows) > 1 {
		return StandardBlockSizeResult{}, ErrInvalidEvidence
	}
	rows := make([]StandardBlockSizeRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		value := row[index["BLOCK_SIZE_BYTES"]]
		if !value.Null {
			bytes, err := strconv.ParseUint(value.Text, 10, 64)
			if err != nil || bytes == 0 {
				return StandardBlockSizeResult{}, ErrInvalidEvidence
			}
		}
		rows = append(rows, StandardBlockSizeRow{Bytes: value})
	}
	return StandardBlockSizeResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadStandardBlockSize reads on one admitted handle. It may try SYSTEM once
// after an empty, NULL or known inaccessible primary. Terminal errors stop it.
func ReadStandardBlockSize(ctx context.Context, reader Reader, request StandardBlockSizeRequest) (StandardBlockSizeResult, error) {
	primary, err := standardBlockSizePlan(request.Options)
	if err != nil {
		return StandardBlockSizeResult{}, err
	}
	if !validDatasetReader(reader) || request.Reference.Validate() != nil {
		return StandardBlockSizeResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return StandardBlockSizeResult{}, err
	}

	result, attempt, err := readStandardBlockSizeSource(ctx, reader, primary, request.Reference, limits)
	result.Attempts = []StandardBlockSizeAttempt{attempt}
	if attempt.Outcome == "available" || attempt.Outcome == "failed" ||
		!request.AllowSystemTablespaceFallback || attempt.Source != StandardBlockSizeParameter {
		return result, err
	}

	fallbackOptions := request.Options
	fallbackOptions.Source = StandardBlockSizeSystemTablespace
	fallback, planErr := standardBlockSizePlan(fallbackOptions)
	if planErr != nil {
		return StandardBlockSizeResult{Attempts: result.Attempts}, errors.Join(err, planErr)
	}
	alternate, second, altErr := readStandardBlockSizeSource(ctx, reader, fallback, request.Reference, limits)
	attempts := append(result.Attempts, second)
	if altErr != nil || (err != nil && second.Outcome != "available") {
		return StandardBlockSizeResult{Attempts: attempts}, errors.Join(err, altErr)
	}
	if second.Outcome == "available" || (attempt.Outcome == "no_rows" && second.Outcome == "null") {
		alternate.Attempts = attempts
		return alternate, nil
	}
	result.Attempts = attempts
	return result, nil
}

func readStandardBlockSizeSource(ctx context.Context, reader Reader, plan datasetSelection, ref reference.Reference, limits rowset.Limits) (StandardBlockSizeResult, StandardBlockSizeAttempt, error) {
	attempt := StandardBlockSizeAttempt{Source: StandardBlockSizeSource(plan.provenance.Source), Outcome: "failed"}
	if err := datasetReadError(ctx, plan, nil); err != nil {
		return StandardBlockSizeResult{}, attempt, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql})
	if err != nil && ctx.Err() == nil {
		switch projection.SourceAccessClass(err) {
		case projection.ErrSourceInaccessible:
			attempt.Outcome = "inaccessible"
		case projection.ErrSourceUnavailable:
			attempt.Outcome = "unavailable"
		}
	}
	if err = datasetReadError(ctx, plan, err); err != nil {
		return StandardBlockSizeResult{}, attempt, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return StandardBlockSizeResult{}, attempt, datasetReadError(ctx, plan, rowset.ErrCapacity)
	}
	facts, decodeErr := decodeStandardBlockSize(result, plan)
	if err = datasetReadError(ctx, plan, decodeErr); err != nil {
		return StandardBlockSizeResult{}, attempt, err
	}
	facts.Provenance.Complete = true
	switch {
	case len(facts.Rows) == 0:
		attempt.Outcome = "no_rows"
	case facts.Rows[0].Bytes.Null:
		attempt.Outcome = "null"
	default:
		attempt.Outcome = "available"
	}
	return facts, attempt, nil
}

// StandardBlockSizeRows transfers the actual cursor for the explicit source.
func StandardBlockSizeRows(ctx context.Context, q rowset.Queryer, options StandardBlockSizeOptions) (*sql.Rows, error) {
	plan, err := standardBlockSizePlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, plan.sql)
	if err != nil {
		return nil, &DatasetError{plan.provenance.Source, err}
	}
	return rows, nil
}

// StandardBlockSizeJSON owns one complete bounded cursor for the explicit source.
func StandardBlockSizeJSON(ctx context.Context, q rowset.Queryer, options StandardBlockSizeOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := standardBlockSizePlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, plan, limits, func(result session.Result) error {
		_, err := decodeStandardBlockSize(result, plan)
		return err
	})
}
