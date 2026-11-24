package asm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const diskGroupCapacitySource = "V$ASM_DISKGROUP_STAT"

// DiskGroupCapacityOptions admits the observed numeric release and the role of
// the caller's already-admitted instance.
type DiskGroupCapacityOptions struct {
	OracleVersion string
	SourceRole    SourceRole
}

// DiskGroupCapacityRequest adds the caller's SQL reference and buffered bounds.
type DiskGroupCapacityRequest struct {
	Options   DiskGroupCapacityOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// DiskGroupCapacity preserves raw state and redundancy, SQL NULL, and exact
// Oracle MB text. UsableFileMB can be negative. Zero can also be a documented
// placeholder for an unmounted group or unsupported redundancy calculation.
// ContainerID is absent before 12c; the view reports zero for this field.
type DiskGroupCapacity struct {
	GroupNumber          session.Value
	Name                 session.Value
	State                session.Value
	Redundancy           session.Value
	TotalMB              session.Value
	FreeMB               session.Value
	RequiredMirrorFreeMB session.Value
	UsableFileMB         session.Value
	ContainerID          *session.Value
}

// DiskGroupCapacityResult retains the rows visible on the admitted instance.
// Complete attests to the selected transport's completion.
type DiskGroupCapacityResult struct {
	Provenance Provenance
	Rows       []DiskGroupCapacity
}

// DiskGroupCapacityStatement selects the local non-discovering ASM STAT view.
func DiskGroupCapacityStatement(options DiskGroupCapacityOptions) (string, error) {
	p, err := diskGroupCapacityPlan(options)
	return p.sql, err
}

func diskGroupCapacityPlan(options DiskGroupCapacityOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	version, _ := oracleversion.Parse(options.OracleVersion)
	if !ok || major < 10 || (major == 10 && version[1] != 2) {
		return selection{}, ErrUnsupportedVersion
	}
	var scope string
	switch options.SourceRole {
	case DatabaseInstance:
		scope = "database_visible_asm_disk_groups"
	case ASMInstance:
		scope = "asm_instance_disk_groups"
	default:
		return selection{}, ErrInvalidRequest
	}

	const exact = ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS "
	p := selection{
		sql: "SELECT TO_CHAR(v.GROUP_NUMBER" + exact + "GROUP_NUMBER, v.NAME AS NAME, v.STATE AS STATE, v.TYPE AS TYPE, " +
			"TO_CHAR(v.TOTAL_MB" + exact + "TOTAL_MB, TO_CHAR(v.FREE_MB" + exact + "FREE_MB, " +
			"TO_CHAR(v.REQUIRED_MIRROR_FREE_MB" + exact + "REQUIRED_MIRROR_FREE_MB, TO_CHAR(v.USABLE_FILE_MB" + exact + "USABLE_FILE_MB",
		schema: []rowset.Column{
			{Name: "GROUP_NUMBER", Kind: rowset.Text},
			{Name: "NAME", Kind: rowset.Text},
			{Name: "STATE", Kind: rowset.Text},
			{Name: "TYPE", Kind: rowset.Text},
			{Name: "TOTAL_MB", Kind: rowset.Text},
			{Name: "FREE_MB", Kind: rowset.Text},
			{Name: "REQUIRED_MIRROR_FREE_MB", Kind: rowset.Text},
			{Name: "USABLE_FILE_MB", Kind: rowset.Text},
		},
		integerText: []string{"GROUP_NUMBER", "TOTAL_MB", "FREE_MB", "REQUIRED_MIRROR_FREE_MB"},
		provenance:  Provenance{Source: diskGroupCapacitySource, OracleMajor: major, Scope: scope, SourceRole: string(options.SourceRole)},
	}
	if major >= 12 {
		p.sql += ", v.CON_ID AS CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM " + diskGroupCapacitySource + " v ORDER BY v.GROUP_NUMBER, v.NAME"
	return p, nil
}

// DecodeDiskGroupCapacity validates the selected projection and all rows.
func DecodeDiskGroupCapacity(result session.Result, options DiskGroupCapacityOptions) (DiskGroupCapacityResult, error) {
	p, err := diskGroupCapacityPlan(options)
	if err != nil {
		return DiskGroupCapacityResult{}, err
	}
	return decodeDiskGroupCapacity(result, p)
}

func decodeDiskGroupCapacity(result session.Result, p selection) (DiskGroupCapacityResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return DiskGroupCapacityResult{}, err
	}
	rows := make([]DiskGroupCapacity, len(result.Rows))
	for i, row := range result.Rows {
		item := DiskGroupCapacity{
			GroupNumber: row[index["GROUP_NUMBER"]], Name: row[index["NAME"]], State: row[index["STATE"]], Redundancy: row[index["TYPE"]],
			TotalMB: row[index["TOTAL_MB"]], FreeMB: row[index["FREE_MB"]],
			RequiredMirrorFreeMB: row[index["REQUIRED_MIRROR_FREE_MB"]], UsableFileMB: row[index["USABLE_FILE_MB"]],
			ContainerID: optional(row, index, "CON_ID"),
		}
		if !item.UsableFileMB.Null && !numeric.IsSignedInteger(item.UsableFileMB.Text) {
			return DiskGroupCapacityResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		if item.ContainerID != nil && !item.ContainerID.Null && item.ContainerID.Text != "0" {
			return DiskGroupCapacityResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = item
	}
	return DiskGroupCapacityResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadDiskGroupCapacity reads one complete bounded result on the admitted handle.
func ReadDiskGroupCapacity(ctx context.Context, reader Reader, request DiskGroupCapacityRequest) (DiskGroupCapacityResult, error) {
	p, err := diskGroupCapacityPlan(request.Options)
	if err != nil {
		return DiskGroupCapacityResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return DiskGroupCapacityResult{}, err
	}
	// A typed nil adapter can satisfy Reader while having no callable query.
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Invalid:
		return DiskGroupCapacityResult{}, ErrInvalidRequest
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return DiskGroupCapacityResult{}, ErrInvalidRequest
		}
	}
	result, err := query(ctx, reader, request.Reference, p)
	if err != nil {
		return DiskGroupCapacityResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return DiskGroupCapacityResult{}, &Error{p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err())}
	}
	facts, decodeErr := decodeDiskGroupCapacity(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return DiskGroupCapacityResult{}, &Error{p.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// DiskGroupCapacityRows returns the actual caller-native cursor.
func DiskGroupCapacityRows(ctx context.Context, q rowset.Queryer, options DiskGroupCapacityOptions) (*sql.Rows, error) {
	p, err := diskGroupCapacityPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// DiskGroupCapacityJSON consumes a separate complete bounded cursor.
func DiskGroupCapacityJSON(ctx context.Context, q rowset.Queryer, options DiskGroupCapacityOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := diskGroupCapacityPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeDiskGroupCapacity(result, p)
		return err
	})
}
