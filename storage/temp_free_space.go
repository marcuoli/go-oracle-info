package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// TempFreeSpaceRequest supplies an observed numeric release and caller reference.
type TempFreeSpaceRequest struct {
	OracleVersion string
	Reference     reference.Reference
}

// TempSpace is one observed temporary tablespace row. Byte counts are exact
// nonnegative decimal text or SQL NULL. FreeSpace includes allocated space
// available for reuse and unallocated space within the observed size.
// ReportingInstanceID and SourceContainerID identify the executing session.
// OwningInstanceID is the view's INST_ID, independent of the reporter.
// A nil pointer means the release projection omitted that column.
type TempSpace struct {
	TablespaceName      session.Value
	TablespaceSize      session.Value
	AllocatedSpace      session.Value
	FreeSpace           session.Value
	ReportingInstanceID session.Value
	SourceContainerID   *session.Value
	Shared              *session.Value
	OwningInstanceID    *session.Value
}

// TempFreeSpaceResult preserves the selected rows, order and duplicates within
// the admitted container. Complete describes the transport, not RAC coverage.
type TempFreeSpaceResult struct {
	Provenance Provenance
	Rows       []TempSpace
}

const tempFreeSpaceSource = "DBA_TEMP_FREE_SPACE"

// TempFreeSpaceStatement selects the documented projection for the observed
// numeric release, without probing capabilities or changing containers.
func TempFreeSpaceStatement(oracleVersion string) (string, error) {
	p, err := tempFreeSpacePlan(oracleVersion)
	return p.sql, err
}

func tempFreeSpacePlan(version string) (selection, error) {
	major, local, err := tempFreeSpaceProfile(version)
	if err != nil {
		return selection{}, err
	}

	// NUMBER can exceed exact float or signed integer capacity. Convert byte
	// counts to decimal text on the database, independently of session NLS.
	projection := "v.TABLESPACE_NAME AS TABLESPACE_NAME"
	schema := []rowset.Column{{Name: "TABLESPACE_NAME", Kind: rowset.Text}}
	counts := []string{"TABLESPACE_SIZE", "ALLOCATED_SPACE", "FREE_SPACE"}
	for _, name := range counts {
		projection += ", TO_CHAR(v." + name + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS " + name
		schema = append(schema, rowset.Column{Name: name, Kind: rowset.Text})
	}
	projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID"
	schema = append(schema, rowset.Column{Name: "SOURCE_INST_ID", Kind: rowset.Integer})
	provenance := Provenance{Source: tempFreeSpaceSource, OracleMajor: major, Scope: "current_container_dictionary"}
	if major >= 12 {
		projection += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		schema = append(schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
		provenance.OptionalColumns = append(provenance.OptionalColumns, "SOURCE_CON_ID")
	}
	order := "v.TABLESPACE_NAME"
	if local {
		projection += ", v.SHARED AS SHARED, v.INST_ID AS OWNING_INST_ID"
		schema = append(schema, rowset.Column{Name: "SHARED", Kind: rowset.Text}, rowset.Column{Name: "OWNING_INST_ID", Kind: rowset.Integer})
		provenance.OptionalColumns = append(provenance.OptionalColumns, "SHARED", "OWNING_INST_ID")
		order += ", v.SHARED, v.INST_ID"
	}
	return selection{
		sql:         "SELECT " + projection + " FROM " + tempFreeSpaceSource + " v ORDER BY " + order,
		schema:      schema,
		integerText: counts,
		provenance:  provenance,
	}, nil
}

func tempFreeSpaceProfile(version string) (major int, local bool, err error) {
	if len(version) > 128 {
		return 0, false, ErrUnsupportedVersion
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return 0, false, ErrUnsupportedVersion
	}
	var n [5]int
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return 0, false, ErrUnsupportedVersion
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, false, ErrUnsupportedVersion
		}
		n[i] = value
	}

	// Oracle added the view in 11.1 and ownership columns in 12.2.0.1.
	// The 26ai reference applies to the observed numeric 23 family.
	switch n[0] {
	case 11:
		if n[1] == 1 || n[1] == 2 {
			return 11, false, nil
		}
	case 12:
		switch {
		case n[1] == 1:
			return 12, false, nil
		case n[1] == 2 && (n[2] > 0 || n[3] >= 1):
			return 12, true, nil
		}
	case 18, 19, 21, 23:
		return n[0], true, nil
	}
	return 0, false, ErrUnsupportedVersion
}

// DecodeTempFreeSpace validates the entire supplied projection and every row.
// It preserves source NULL, raw ownership flags and duplicate names.
func DecodeTempFreeSpace(result session.Result, oracleVersion string) (TempFreeSpaceResult, error) {
	p, err := tempFreeSpacePlan(oracleVersion)
	if err != nil {
		return TempFreeSpaceResult{}, err
	}
	return decodeTempFreeSpace(result, p)
}

func decodeTempFreeSpace(result session.Result, p selection) (TempFreeSpaceResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return TempFreeSpaceResult{}, err
	}
	rows := make([]TempSpace, len(result.Rows))
	for i, row := range result.Rows {
		name := row[index["TABLESPACE_NAME"]]
		if name.Null || name.Text == "" {
			return TempFreeSpaceResult{}, &Error{tempFreeSpaceSource, ErrInvalidEvidence}
		}
		rows[i] = TempSpace{
			TablespaceName:      name,
			TablespaceSize:      row[index["TABLESPACE_SIZE"]],
			AllocatedSpace:      row[index["ALLOCATED_SPACE"]],
			FreeSpace:           row[index["FREE_SPACE"]],
			ReportingInstanceID: row[index["SOURCE_INST_ID"]],
			SourceContainerID:   optional(row, index, "SOURCE_CON_ID"),
			Shared:              optional(row, index, "SHARED"),
			OwningInstanceID:    optional(row, index, "OWNING_INST_ID"),
		}
	}
	return TempFreeSpaceResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadTempFreeSpace executes once through an admitted complete bounded reader.
// Errors leave the selection unavailable, distinct from a successful empty read.
func ReadTempFreeSpace(ctx context.Context, reader Reader, req TempFreeSpaceRequest) (TempFreeSpaceResult, error) {
	p, err := tempFreeSpacePlan(req.OracleVersion)
	if err != nil {
		return TempFreeSpaceResult{}, err
	}
	result, err := query(ctx, reader, req.Reference, p)
	if err != nil {
		return TempFreeSpaceResult{}, err
	}
	facts, decodeErr := decodeTempFreeSpace(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return TempFreeSpaceResult{}, &Error{tempFreeSpaceSource, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// TempFreeSpaceRows returns the caller's native cursor for the selected release.
// Callers own iteration, Err, Close, affinity and finite streaming budgets.
func TempFreeSpaceRows(ctx context.Context, q rowset.Queryer, oracleVersion string) (*sql.Rows, error) {
	p, err := tempFreeSpacePlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// TempFreeSpaceJSON independently returns a complete bounded native dataset.
// Byte counts are strings so their exact value survives JSON consumers.
func TempFreeSpaceJSON(ctx context.Context, q rowset.Queryer, oracleVersion string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := tempFreeSpacePlan(oracleVersion)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeTempFreeSpace(r, p)
		return err
	})
}
