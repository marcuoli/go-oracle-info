package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/marcuoli/go-oracle-info/internal/legacyversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ScalarSystemScope controls which admitted container rows are selected on 12c+.
// Both options refer to the visible non-CDB rows before CON_ID existed.
type ScalarSystemScope uint8

const (
	ScalarSystemCurrentAndDatabase ScalarSystemScope = iota
	ScalarSystemDatabaseOnly
)

// ScalarSystemOptions carries the observed numeric version, scope and filters.
// The supplied handle already belongs to the desired database and container.
type ScalarSystemOptions struct {
	OracleVersion string
	Scope         ScalarSystemScope
	Filter        Filter
}

// ScalarSystemRequest adds a caller-owned SQL reference for an admitted reader.
type ScalarSystemRequest struct {
	Options   ScalarSystemOptions
	Reference reference.Reference
}

// ScalarSystemProvenance retains the selected scope and the observed version.
type ScalarSystemProvenance struct {
	Provenance
	OracleVersion string `json:"oracle_version"`
	Complete      bool   `json:"complete"`
}

// ScalarSystemParameter retains raw VALUE, including comma-containing values
// and SQL NULL. Pointer fields are nil only when their source column is absent.
type ScalarSystemParameter struct {
	InstanceID           session.Value
	ContainerID          *session.Value
	Number               session.Value
	Name                 session.Value
	Type                 session.Value
	Value                session.Value
	DisplayValue         *session.Value
	IsDefault            session.Value
	IsSessionModifiable  session.Value
	IsSystemModifiable   session.Value
	IsPDBModifiable      *session.Value
	IsInstanceModifiable *session.Value
	IsModified           session.Value
	IsAdjusted           session.Value
	IsDeprecated         *session.Value
	IsBasic              *session.Value
	Description          session.Value
	UpdateComment        session.Value
}

// ScalarSystemState distinguishes observed rows, successful empty results,
// inaccessible or unavailable sources, failed reads and unsupported releases.
type ScalarSystemState string

const (
	ScalarSystemAvailable    ScalarSystemState = "available"
	ScalarSystemNoRows       ScalarSystemState = "no_rows"
	ScalarSystemInaccessible ScalarSystemState = "inaccessible"
	ScalarSystemUnavailable  ScalarSystemState = "unavailable"
	ScalarSystemFailed       ScalarSystemState = "failed"
	ScalarSystemUnsupported  ScalarSystemState = "unsupported"
)

// ScalarSystemResult preserves selected scalar rows, diagnostics through the
// returned error, and scope. A successful empty result has a nonnil Rows slice.
type ScalarSystemResult struct {
	State      ScalarSystemState
	Provenance ScalarSystemProvenance
	Rows       []ScalarSystemParameter
}

// ErrInvalidScalarSystemScope rejects unknown scope choices before querying.
var ErrInvalidScalarSystemScope = errors.New("invalid scalar system parameter scope")

// ErrSourceInaccessible identifies a query rejected for insufficient privilege.
var ErrSourceInaccessible = projection.ErrSourceInaccessible

// ErrSourceUnavailable identifies an unresolved source or unavailable database
// state. The original query diagnostic remains in the error chain.
var ErrSourceUnavailable = projection.ErrSourceUnavailable

const scalarDatabaseRows = "whole_database_rows"

// ScalarSystemStatement selects the release-aware instance-wide scalar rows.
// VALUE and DISPLAY_VALUE remain independent when both columns are present.
func ScalarSystemStatement(options ScalarSystemOptions) (string, []any, error) {
	plan, err := scalarSystemPlan(options)
	return plan.sql, plan.args, err
}

func scalarSystemPlan(options ScalarSystemOptions) (selection, error) {
	major, ok := legacyversion.Major(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	if options.Scope != ScalarSystemCurrentAndDatabase && options.Scope != ScalarSystemDatabaseOnly {
		return selection{}, ErrInvalidScalarSystemScope
	}

	// Reuse the published projection. 10g adds the display and two flag
	// columns. The basic-parameter flag is first documented in 11g.
	plan, err := systemPlan(max(major, 11), options.Filter)
	if err != nil {
		return selection{}, err
	}
	switch {
	case major < 11:
		columns := slices.DeleteFunc(plan.schema, func(column rowset.Column) bool {
			switch column.Name {
			case "ISBASIC":
				return true
			case "DISPLAY_VALUE", "ISINSTANCE_MODIFIABLE", "ISDEPRECATED":
				return major < 10
			}
			return false
		})
		plan.provenance.OracleMajor = major
		plan = build(major, plan.provenance, columns, nil)
	case major >= 12 && options.Scope == ScalarSystemDatabaseOnly:
		plan.provenance.Scope = scalarDatabaseRows
		plan = build(major, plan.provenance, plan.schema, nil)
	}
	plan.classifyAccess = true
	return plan, nil
}

func scalarSystemFailure(err error) ScalarSystemResult {
	state := ScalarSystemFailed
	if errors.Is(err, ErrUnsupportedVersion) {
		state = ScalarSystemUnsupported
	}
	return ScalarSystemResult{State: state}
}

// DecodeScalarSystem validates the full projection and every row before
// publishing any typed facts. A pure decoder cannot establish transport completion.
func DecodeScalarSystem(result session.Result, options ScalarSystemOptions) (ScalarSystemResult, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return scalarSystemFailure(err), err
	}
	return decodeScalarSystem(result, plan, options.OracleVersion)
}

func decodeScalarSystem(result session.Result, plan selection, version string) (ScalarSystemResult, error) {
	facts := ScalarSystemResult{State: ScalarSystemFailed, Provenance: ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: version}}
	index, err := parameterColumns(result, plan)
	if err != nil {
		return facts, err
	}
	facts.Rows = make([]ScalarSystemParameter, len(result.Rows))
	facts.State = ScalarSystemNoRows
	if len(facts.Rows) > 0 {
		facts.State = ScalarSystemAvailable
	}
	for i, row := range result.Rows {
		facts.Rows[i] = ScalarSystemParameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			Number: row[index["NUM"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Value: row[index["VALUE"]], DisplayValue: optional(row, index, "DISPLAY_VALUE"), IsDefault: row[index["ISDEFAULT"]],
			IsSessionModifiable: row[index["ISSES_MODIFIABLE"]], IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]],
			IsPDBModifiable: optional(row, index, "ISPDB_MODIFIABLE"), IsInstanceModifiable: optional(row, index, "ISINSTANCE_MODIFIABLE"),
			IsModified: row[index["ISMODIFIED"]], IsAdjusted: row[index["ISADJUSTED"]], IsDeprecated: optional(row, index, "ISDEPRECATED"),
			IsBasic: optional(row, index, "ISBASIC"), Description: row[index["DESCRIPTION"]], UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return facts, nil
}

// ReadScalarSystem queries the admitted complete reader once and discards all
// returned rows on query, decode or cancellation errors.
func ReadScalarSystem(ctx context.Context, reader Reader, request ScalarSystemRequest) (ScalarSystemResult, error) {
	plan, err := scalarSystemPlan(request.Options)
	if err != nil {
		return scalarSystemFailure(err), err
	}
	failure := ScalarSystemResult{State: ScalarSystemFailed, Provenance: ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: request.Options.OracleVersion}}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		switch {
		case errors.Is(err, ErrSourceInaccessible):
			failure.State = ScalarSystemInaccessible
		case errors.Is(err, ErrSourceUnavailable):
			failure.State = ScalarSystemUnavailable
		}
		return failure, err
	}
	facts, decodeErr := decodeScalarSystem(result, plan, request.Options.OracleVersion)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return failure, &Error{Source: plan.provenance.Source, cause: err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ScalarSystemRows returns the actual caller-native cursor. The caller owns
// iteration, terminal checks, Close, deadlines and stream budgets.
func ScalarSystemRows(ctx context.Context, q rowset.Queryer, options ScalarSystemOptions) (*sql.Rows, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// ScalarSystemJSON independently executes the same selection and publishes only
// a complete bounded JSON envelope, with version and scope kept as provenance.
func ScalarSystemJSON(ctx context.Context, q rowset.Queryer, options ScalarSystemOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := scalarSystemPlan(options)
	if err != nil {
		return nil, err
	}
	provenance := ScalarSystemProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion, Complete: true}
	return readParameterJSON(ctx, q, plan, provenance, limits)
}
