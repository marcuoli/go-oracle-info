package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// VersionPrecision identifies the selected native column, not inferred patch coverage.
type VersionPrecision string

const (
	LegacyPrecision VersionPrecision = "legacy"
	FullPrecision   VersionPrecision = "full"
)

// VersionRow preserves one qualified visible member observation. VERSION is never
// substituted for missing VERSION_FULL. ContainerID/VersionFull nil means absent
// source column; a nonnil SQL NULL remains a distinct observation. Local session
// context is repeated per row and never describes a remote member session.
// CloudService is projected only on the documented numeric23+ source; NULL
// alone never establishes self-managed admission.
type VersionRow struct {
	CurrentContainerID *session.Value
	CloudService       *session.Value
	SessionUser        session.Value
	IsDBA              session.Value
	InstanceID         session.Value
	ContainerID        *session.Value
	InstanceNumber     session.Value
	Name               session.Value
	HostName           session.Value
	Status             session.Value
	Version            session.Value
	VersionFull        *session.Value
}

// VersionResult retains all source rows including duplicates, mounted members and
// conflicting observations. It certifies neither network nor physical RAC coverage.
type VersionResult struct {
	ContextSource string
	State         State
	Provenance    Provenance
	Precision     VersionPrecision
	Rows          []VersionRow
}

// VersionsStatement selects every qualified visible member without a local-only
// fallback. The caller admits the observed numeric major, container and privileges.
func VersionsStatement(major int) (string, error) {
	plan, err := versionsPlan(major)
	return plan.sql, err
}
func versionsPlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "INSTANCE_NUMBER", Kind: rowset.Integer}}
	for _, name := range []string{"INSTANCE_NAME", "HOST_NAME", "STATUS", "VERSION"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	columns = append(columns, rowset.Column{Name: "SESSION_USER", Kind: rowset.Text}, rowset.Column{Name: "ISDBA", Kind: rowset.Text})
	scope := "visible_qualified_instance_versions"
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		scope = "current_container_and_whole_database_qualified_instance_versions"
	}
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CURRENT_CONTAINER_ID", Kind: rowset.Text})
	}
	if major >= 23 {
		columns = append(columns, rowset.Column{Name: "CLOUD_SERVICE", Kind: rowset.Text})
	}
	if major >= 18 {
		columns = append(columns, rowset.Column{Name: "VERSION_FULL", Kind: rowset.Text})
	}
	aliases := make([]string, len(columns))
	for i, c := range columns {
		switch c.Name {
		case "SESSION_USER", "ISDBA", "CLOUD_SERVICE":
			aliases[i] = "SYS_CONTEXT('USERENV','" + c.Name + "') AS " + c.Name
		case "CURRENT_CONTAINER_ID":
			aliases[i] = "SYS_CONTEXT('USERENV','CON_ID') AS CURRENT_CONTAINER_ID"
		default:
			aliases[i] = "i." + c.Name + " AS " + c.Name
		}
	}
	statement := "SELECT " + strings.Join(aliases, ", ") + " FROM GV$INSTANCE i"
	if major >= 12 {
		statement += " WHERE i.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))"
	}
	statement += " ORDER BY i.INST_ID"
	if major >= 12 {
		statement += ", i.CON_ID"
	}
	statement += ", i.INSTANCE_NUMBER"
	return selection{sql: statement, schema: columns, provenance: Provenance{Source: "GV$INSTANCE", OracleMajor: major, Scope: scope}}, nil
}

// DecodeVersions validates every row before returning any source facts. A decoder
// cannot establish transport completion or the caller's admission to the source.
func DecodeVersions(result session.Result, major int) (VersionResult, error) {
	plan, err := versionsPlan(major)
	if err != nil {
		return VersionResult{State: Unavailable}, err
	}
	unavailable := VersionResult{State: Unavailable, Provenance: plan.provenance}
	fail := func() (VersionResult, error) {
		return unavailable, &Error{Operation: "decode versions", cause: ErrInvalidEvidence}
	}
	index, err := columns(result, plan.schema)
	if err != nil {
		return fail()
	}
	rows := make([]VersionRow, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = VersionRow{InstanceID: row[index["INST_ID"]], InstanceNumber: row[index["INSTANCE_NUMBER"]], Name: row[index["INSTANCE_NAME"]], HostName: row[index["HOST_NAME"]], SessionUser: row[index["SESSION_USER"]], IsDBA: row[index["ISDBA"]], Status: row[index["STATUS"]], Version: row[index["VERSION"]]}
		if p, ok := index["CURRENT_CONTAINER_ID"]; ok {
			v := row[p]
			rows[i].CurrentContainerID = &v
		}
		if p, ok := index["CLOUD_SERVICE"]; ok {
			v := row[p]
			rows[i].CloudService = &v
		}
		if p, ok := index["CON_ID"]; ok {
			v := row[p]
			rows[i].ContainerID = &v
		}
		if p, ok := index["VERSION_FULL"]; ok {
			v := row[p]
			rows[i].VersionFull = &v
		}
	}
	state := Available
	if len(rows) == 0 {
		state = NoRows
	}
	precision := LegacyPrecision
	if major >= 18 {
		precision = FullPrecision
	}
	return VersionResult{ContextSource: "SYS_CONTEXT('USERENV')", State: state, Provenance: plan.provenance, Precision: precision, Rows: rows}, nil
}

// ReadVersions executes once on a complete-result reader. Any query, terminal,
// decode or post-query cancellation error discards all returned member rows.
func ReadVersions(ctx context.Context, reader Reader, request Request) (VersionResult, error) {
	plan, err := versionsPlan(request.OracleMajor)
	if err != nil {
		return VersionResult{State: Unavailable}, err
	}
	unavailable := VersionResult{State: Unavailable, Provenance: plan.provenance}
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || request.Reference.Validate() != nil {
		return unavailable, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return unavailable, &Error{Operation: "query versions", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: plan.sql})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "query versions", cause: err}
	}
	facts, err := DecodeVersions(result, request.OracleMajor)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "read versions", cause: err}
	}
	return facts, nil
}

// VersionsRows returns the actual caller-native cursor; its caller owns budgets,
// iteration, Err and Close. No connection, lease or synthesized rows are created.
func VersionsRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := versionsPlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// VersionsJSONEnvelope retains actual version precision and the separately local
// execution-context source beside native ordered member rows.
type VersionsJSONEnvelope struct {
	Provenance    Provenance       `json:"provenance"`
	ContextSource string           `json:"context_source"`
	Precision     VersionPrecision `json:"precision"`
	Data          json.RawMessage  `json:"data"`
}

// VersionsJSON independently consumes the same complete native source through
// session/rowset. Limits include provenance; any failure publishes no JSON.
func VersionsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := versionsPlan(major)
	if err != nil {
		return nil, err
	}
	precision := LegacyPrecision
	if major >= 18 {
		precision = FullPrecision
	}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql), plan.schema, limits, rowset.EnvelopeOptions{
		Header: struct {
			Provenance    Provenance       `json:"provenance"`
			ContextSource string           `json:"context_source"`
			Precision     VersionPrecision `json:"precision"`
		}{Provenance: plan.provenance, ContextSource: "SYS_CONTEXT('USERENV')", Precision: precision},
		Validate: projection.ValidateRowset(&Error{Operation: "validate versions", cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := DecodeVersions(result, major)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "query":
			return &Error{Operation: "query", cause: cause}
		case "encode", "budget", "consume":
			return &Error{Operation: stage + " versions", cause: cause}
		}
		return cause
	})
}
