package storage

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

// BlockCorruptionsRequest supplies an observed numeric release and caller SQL
// reference. The caller owns the admitted session, privileges and read budget.
type BlockCorruptionsRequest struct {
	OracleVersion string
	Reference     reference.Reference
}

// BlockCorruption represents one recorded range. FileID is Oracle's absolute
// file number and BlockID is its first block. CorruptionChange preserves the
// source change number, including zero for media corruption. Numeric values
// are exact decimal text or SQL NULL. A nil ContainerID means unprojected.
type BlockCorruption struct {
	FileID, BlockID, Blocks, CorruptionChange, CorruptionType session.Value
	ContainerID                                               *session.Value
}

// BlockCorruptionsResult preserves recorded ranges in the connected view.
// Complete confirms the read. An empty inventory gives no validation of blocks
// or coverage beyond the caller's visible scope.
type BlockCorruptionsResult struct {
	Provenance Provenance
	Rows       []BlockCorruption
}

const blockCorruptionsSource = "V$DATABASE_BLOCK_CORRUPTION"

// BlockCorruptionsStatement selects the documented shape for the observed
// release. The selection always reads the caller's visible Oracle records.
func BlockCorruptionsStatement(oracleVersion string) (string, error) {
	plan, err := blockCorruptionsPlan(oracleVersion)
	return plan.sql, err
}

func blockCorruptionsPlan(version string) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(version)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	// Use decimal text at the Oracle boundary so native and JSON consumers can
	// carry a change number larger than the exact range of a floating point value.
	const integerFormat = ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS "
	projection := "TO_CHAR(c.FILE#" + integerFormat + "FILE_ID, " +
		"TO_CHAR(c.BLOCK#" + integerFormat + "BLOCK_ID, " +
		"TO_CHAR(c.BLOCKS" + integerFormat + "BLOCKS, " +
		"TO_CHAR(c.CORRUPTION_CHANGE#" + integerFormat + "CORRUPTION_CHANGE, " +
		"c.CORRUPTION_TYPE AS CORRUPTION_TYPE"
	schema := []rowset.Column{
		{Name: "FILE_ID", Kind: rowset.Text},
		{Name: "BLOCK_ID", Kind: rowset.Text},
		{Name: "BLOCKS", Kind: rowset.Text},
		{Name: "CORRUPTION_CHANGE", Kind: rowset.Text},
		{Name: "CORRUPTION_TYPE", Kind: rowset.Text},
	}
	provenance := Provenance{Source: blockCorruptionsSource, Scope: "connected_view_recorded_corruption", OracleMajor: major}
	order := "c.FILE#, c.BLOCK#, c.BLOCKS, c.CORRUPTION_CHANGE#, c.CORRUPTION_TYPE"
	if major >= 12 {
		projection += ", c.CON_ID AS CON_ID"
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		provenance.OptionalColumns = []string{"CON_ID"}
		order = "c.CON_ID, " + order
	}
	return selection{
		sql:         "SELECT " + projection + " FROM " + blockCorruptionsSource + " c ORDER BY " + order,
		schema:      schema,
		integerText: []string{"FILE_ID", "BLOCK_ID", "BLOCKS", "CORRUPTION_CHANGE"},
		provenance:  provenance,
	}, nil
}

// DecodeBlockCorruptions validates the selected schema and every row. It
// preserves SQL NULLs, source corruption labels and all recorded ranges.
func DecodeBlockCorruptions(result session.Result, oracleVersion string) (BlockCorruptionsResult, error) {
	plan, err := blockCorruptionsPlan(oracleVersion)
	if err != nil {
		return BlockCorruptionsResult{}, err
	}
	return decodeBlockCorruptions(result, plan)
}

func decodeBlockCorruptions(result session.Result, plan selection) (BlockCorruptionsResult, error) {
	index, err := columns(result, plan)
	if err != nil {
		return BlockCorruptionsResult{}, err
	}
	rows := make([]BlockCorruption, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = BlockCorruption{
			FileID:           row[index["FILE_ID"]],
			BlockID:          row[index["BLOCK_ID"]],
			Blocks:           row[index["BLOCKS"]],
			CorruptionChange: row[index["CORRUPTION_CHANGE"]],
			CorruptionType:   row[index["CORRUPTION_TYPE"]],
			ContainerID:      optional(row, index, "CON_ID"),
		}
	}
	return BlockCorruptionsResult{Provenance: plan.provenance, Rows: rows}, nil
}

// ReadBlockCorruptions executes once on the caller's admitted reader. A read,
// decode or cancellation failure discards the result in full.
func ReadBlockCorruptions(ctx context.Context, reader Reader, request BlockCorruptionsRequest) (BlockCorruptionsResult, error) {
	plan, err := blockCorruptionsPlan(request.OracleVersion)
	if err != nil {
		return BlockCorruptionsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, plan)
	if err != nil {
		return BlockCorruptionsResult{}, err
	}
	facts, decodeErr := decodeBlockCorruptions(result, plan)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return BlockCorruptionsResult{}, &Error{blockCorruptionsSource, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// BlockCorruptionsRows returns the caller's actual native cursor. The caller
// owns iteration, Err, Close, affinity and finite streaming budgets.
func BlockCorruptionsRows(ctx context.Context, q rowset.Queryer, oracleVersion string) (*sql.Rows, error) {
	plan, err := blockCorruptionsPlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// BlockCorruptionsJSON publishes the complete bounded dataset from a separate
// cursor. File, block, count and change number are exact JSON strings.
func BlockCorruptionsJSON(ctx context.Context, q rowset.Queryer, oracleVersion string, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := blockCorruptionsPlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, plan, limits, func(result session.Result) error {
		_, err := decodeBlockCorruptions(result, plan)
		return err
	})
}
