package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MemberStartupOptions carries the observed numeric release of the admitted connection.
type MemberStartupOptions struct{ OracleVersion string }

// MemberStartupRequest supplies the caller reference and buffered result limits.
type MemberStartupRequest struct {
	Options   MemberStartupOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// MemberStartupRow retains a reporting member ID and its civil startup time and
// raw instance status. StartupTime has whole seconds and no timezone.
type MemberStartupRow struct {
	InstanceID, StartupTime, Status session.Value
}

// MemberStartupResult retains visible observations in source order, including
// duplicates. Complete describes the transport, not physical RAC membership.
type MemberStartupResult struct {
	State      State
	Provenance VisibilityProvenance
	Rows       []MemberStartupRow
}

// MemberStartupStatement selects visible instance startup and status observations.
func MemberStartupStatement(options MemberStartupOptions) (string, error) {
	plan, err := memberStartupPlan(options)
	return plan.sql, err
}

func memberStartupPlan(options MemberStartupOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	return selection{
		sql: "SELECT i.INST_ID AS INST_ID, " + datetime.GregorianDate("i.STARTUP_TIME") +
			" AS STARTUP_TIME, i.STATUS AS STATUS FROM SYS.GV_$INSTANCE i ORDER BY i.INST_ID",
		schema: []rowset.Column{
			{Name: "INST_ID", Kind: rowset.Integer},
			{Name: "STARTUP_TIME", Kind: rowset.Text},
			{Name: "STATUS", Kind: rowset.Text},
		},
		provenance:     Provenance{Source: "GV$INSTANCE", OracleMajor: major, Scope: "visible_reporting_members"},
		classifyAccess: true,
	}, nil
}

// DecodeMemberStartup validates every supplied row without claiming that the
// transport completed.
func DecodeMemberStartup(result session.Result, options MemberStartupOptions) (MemberStartupResult, error) {
	plan, err := memberStartupPlan(options)
	if err != nil {
		return MemberStartupResult{State: Unsupported}, err
	}
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion}
	return decodeMemberStartup(result, plan, p)
}

func decodeMemberStartup(result session.Result, plan selection, p VisibilityProvenance) (MemberStartupResult, error) {
	facts := MemberStartupResult{State: Failed, Provenance: p}
	index, err := columns(result, plan.schema)
	if err != nil {
		return facts, &Error{Operation: "decode", cause: err}
	}
	const civil = "2006-01-02T15:04:05"
	rows := make([]MemberStartupRow, len(result.Rows))
	for i, row := range result.Rows {
		startup := row[index["STARTUP_TIME"]]
		if !startup.Null {
			if _, err := time.Parse(civil, startup.Text); err != nil || len(startup.Text) != len(civil) {
				return facts, &Error{Operation: "decode", cause: ErrInvalidEvidence}
			}
		}
		rows[i] = MemberStartupRow{InstanceID: row[index["INST_ID"]], StartupTime: startup, Status: row[index["STATUS"]]}
	}
	facts.State = NoRows
	if len(rows) != 0 {
		facts.State = Available
	}
	facts.Rows = rows
	return facts, nil
}

// ReadMemberStartup reads once and exposes no partial member observations.
func ReadMemberStartup(ctx context.Context, reader Reader, request MemberStartupRequest) (MemberStartupResult, error) {
	plan, err := memberStartupPlan(request.Options)
	if err != nil {
		return MemberStartupResult{State: Unsupported}, err
	}
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: request.Options.OracleVersion}
	failure := MemberStartupResult{State: Failed, Provenance: p}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return failure, err
	}
	result, err := queryInventory(ctx, reader, request.Reference, plan)
	if err != nil {
		switch {
		case errors.Is(err, ErrSourceInaccessible):
			failure.State = Inaccessible
		case errors.Is(err, ErrSourceUnavailable):
			failure.State = Unavailable
		}
		return failure, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return failure, &Error{Operation: "budget", cause: rowset.ErrCapacity}
	}
	facts, err := decodeMemberStartup(result, plan, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return failure, &Error{Operation: "read", cause: err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// MemberStartupRows transfers the actual caller-native cursor.
func MemberStartupRows(ctx context.Context, q rowset.Queryer, options MemberStartupOptions) (*sql.Rows, error) {
	plan, err := memberStartupPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// MemberStartupJSON independently consumes and validates a bounded cursor.
func MemberStartupJSON(ctx context.Context, q rowset.Queryer, options MemberStartupOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := memberStartupPlan(options)
	if err != nil {
		return nil, err
	}
	p := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion, Complete: true}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: p,
		Validate: projection.ValidateRowset(ErrInvalidEvidence, func(r session.Result) error {
			_, err := decodeMemberStartup(r, plan, p)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "limits":
			return cause
		case "query":
			return projection.ClassifySourceError(&Error{Operation: stage, cause: cause})
		default:
			return &Error{Operation: stage, cause: cause}
		}
	})
}
