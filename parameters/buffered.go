package parameters

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Reader supplies complete bounded results, retaining terminal/close failures.
// A session-owned reader keeps its own lease, cancellation and transport policy.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits the known source version, filters and caller-owned SQL reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

// SPFileRequest keeps stored SID clauses separate from reporting instances.
type SPFileRequest struct {
	OracleMajor int
	Filter      SPFileFilter
	Reference   reference.Reference
}

var (
	// ErrInvalidRequest rejects an absent reader or invalid SQL reference.
	ErrInvalidRequest = errors.New("parameter reader and valid reference are required")
	// ErrInvalidEvidence rejects malformed projections, exact values or selections.
	ErrInvalidEvidence = errors.New("invalid Oracle parameter evidence")
)

// Parameter retains each effective list-element row without normalizing flags.
// Numeric fields retain exact integer text in session.Value. ContainerID and
// IsPDBModifiable are nil only when their columns were not selected on 11g;
// projected SQL NULL is a nonnil value with Null=true. Value/DisplayValue are text.
type Parameter struct {
	InstanceID           session.Value
	ContainerID          *session.Value
	Number               session.Value
	Name                 session.Value
	Type                 session.Value
	Ordinal              session.Value
	Value                session.Value
	DisplayValue         session.Value
	IsDefault            session.Value
	IsSessionModifiable  session.Value
	IsSystemModifiable   session.Value
	IsPDBModifiable      *session.Value
	IsInstanceModifiable session.Value
	IsModified           session.Value
	IsAdjusted           session.Value
	IsDeprecated         session.Value
	IsBasic              session.Value
	Description          session.Value
	UpdateComment        session.Value
}

// EffectiveResult preserves complete selected row order, duplicates and provenance.
// A successful empty Rows slice is not evidence of unobserved parameter defaults.
type EffectiveResult struct {
	Provenance Provenance
	Rows       []Parameter
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

// DecodeEffective validates the entire admitted projection before publishing rows.
func DecodeEffective(result session.Result, major int, filter Filter) (EffectiveResult, error) {
	plan, err := effectivePlan(major, filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	return decodeEffective(result, plan)
}

// DecodeHidden validates a complete visible-underscore result in effective shape.
// Missing requested names are left absent; NULL/default flags are not fabricated.
func DecodeHidden(result session.Result, major int, filter Filter) (EffectiveResult, error) {
	plan, err := hiddenPlan(major, filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	return decodeEffective(result, plan)
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

// ReadEffective queries once through the admitted reader, with no connection or
// retry ownership, then decodes only a complete successful result.
func ReadEffective(ctx context.Context, reader Reader, request Request) (EffectiveResult, error) {
	plan, err := effectivePlan(request.OracleMajor, request.Filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	facts, decodeErr := decodeEffective(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return EffectiveResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
}

// ReadHidden independently reads visible underscore instance facts. Report
// nondefault/explicit-name unions belong to callers, including peer comparison.
func ReadHidden(ctx context.Context, reader Reader, request Request) (EffectiveResult, error) {
	plan, err := hiddenPlan(request.OracleMajor, request.Filter)
	if err != nil {
		return EffectiveResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	facts, decodeErr := decodeEffective(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return EffectiveResult{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return facts, nil
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

func queryParameters(ctx context.Context, reader Reader, ref reference.Reference, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql, Args: plan.args})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{Source: plan.provenance.Source, cause: err}
	}
	return result, nil
}

func decodeEffective(result session.Result, plan selection) (EffectiveResult, error) {
	index, err := parameterColumns(result, plan)
	if err != nil {
		return EffectiveResult{}, err
	}
	rows := make([]Parameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = Parameter{
			InstanceID: row[index["INST_ID"]], ContainerID: optional(row, index, "CON_ID"),
			Number: row[index["NUM"]], Name: row[index["NAME"]], Type: row[index["TYPE"]],
			Ordinal: row[index["ORDINAL"]], Value: row[index["VALUE"]], DisplayValue: row[index["DISPLAY_VALUE"]],
			IsDefault: row[index["ISDEFAULT"]], IsSessionModifiable: row[index["ISSES_MODIFIABLE"]],
			IsSystemModifiable: row[index["ISSYS_MODIFIABLE"]], IsPDBModifiable: optional(row, index, "ISPDB_MODIFIABLE"),
			IsInstanceModifiable: row[index["ISINSTANCE_MODIFIABLE"]], IsModified: row[index["ISMODIFIED"]],
			IsAdjusted: row[index["ISADJUSTED"]], IsDeprecated: row[index["ISDEPRECATED"]], IsBasic: row[index["ISBASIC"]],
			Description: row[index["DESCRIPTION"]], UpdateComment: row[index["UPDATE_COMMENT"]],
		}
	}
	return EffectiveResult{Provenance: plan.provenance, Rows: rows}, nil
}

func optional(row []session.Value, index map[string]int, name string) *session.Value {
	i, present := index[name]
	if !present {
		return nil
	}
	value := row[i]
	return &value
}

func parameterColumns(result session.Result, plan selection) (map[string]int, error) {
	fail := func() (map[string]int, error) {
		return nil, &Error{Source: plan.provenance.Source, cause: ErrInvalidEvidence}
	}
	names := make([]string, len(plan.schema))
	for i, column := range plan.schema {
		names[i] = column.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return fail()
	}
	for _, row := range result.Rows {
		for _, column := range plan.schema {
			value := row[index[column.Name]]
			if !utf8.ValidString(value.Text) || (value.Null && value.Text != "") {
				return fail()
			}
			if column.Kind == rowset.Integer && !value.Null {
				if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
					return fail()
				}
			}
		}
		name := row[index["NAME"]]
		if plan.provenance.NameClass != "" && (name.Null || !strings.HasPrefix(name.Text, "_")) {
			return fail()
		}
		if !strings.Contains(strings.ToLower(name.Text), strings.ToLower(plan.provenance.NameContains)) {
			return fail()
		}
		if !admittedText(name, plan.provenance.Names) {
			return fail()
		}
		if len(plan.provenance.Instances) > 0 {
			id := row[index["INST_ID"]]
			number, err := strconv.ParseInt(id.Text, 10, 64)
			found := false
			for _, wanted := range plan.provenance.Instances {
				found = found || wanted == number
			}
			if id.Null || err != nil || !found {
				return fail()
			}
		}
		if len(plan.provenance.SIDs) > 0 && !admittedText(row[index["SID"]], plan.provenance.SIDs) {
			return fail()
		}
	}
	return index, nil
}

func admittedText(value session.Value, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	if value.Null {
		return false
	}
	for _, wanted := range allowed {
		if value.Text == wanted {
			return true
		}
	}
	return false
}
