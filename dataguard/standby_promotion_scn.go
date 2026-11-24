package dataguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// StandbyPromotion preserves the raw V$DATABASE promotion SCN. Exact zero and
// SQL NULL remain distinct, with no failover feasibility inferred.
type StandbyPromotion struct {
	SCN                            session.Value
	ContainerID, SourceContainerID *session.Value
}

// StandbyPromotionSCNResult contains the admitted source rows and their provenance.
type StandbyPromotionSCNResult struct {
	Provenance Provenance
	Rows       []StandbyPromotion
}

// StandbyPromotionSCNStatement selects documented recovery identity fields.
func StandbyPromotionSCNStatement(version string) (string, error) {
	p, err := standbyPromotionSCNPlan(version)
	return p.sql, err
}

func standbyPromotionSCNPlan(version string) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(version)
	if !ok || major < 10 {
		return selection{}, ErrUnsupportedVersion
	}
	fields := []field{exact("STANDBY_BECAME_PRIMARY_SCN", "v.STANDBY_BECAME_PRIMARY_SCN")}
	if major >= 12 {
		fields = appendRecoveryContainer(fields)
	}
	p, err := selectAdmittedView(version, Filter{}, "V$DATABASE", fields, "", "")
	p.provenance.Scope = "connected_database"
	p.single = true
	return p, err
}

// DecodeStandbyPromotionSCN preserves the exact values and SQL NULLs of every selected row.
func DecodeStandbyPromotionSCN(result session.Result, version string) (StandbyPromotionSCNResult, error) {
	p, err := standbyPromotionSCNPlan(version)
	if err != nil {
		return StandbyPromotionSCNResult{}, err
	}
	return decodeStandbyPromotionSCN(result, p)
}

func decodeStandbyPromotionSCN(result session.Result, p selection) (StandbyPromotionSCNResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return StandbyPromotionSCNResult{}, err
	}
	rows := make([]StandbyPromotion, len(result.Rows))
	for i, row := range result.Rows {
		v := StandbyPromotion{
			SCN:         row[index["STANDBY_BECAME_PRIMARY_SCN"]],
			ContainerID: recoveryOptional(row, index, "CON_ID"), SourceContainerID: recoveryOptional(row, index, "SOURCE_CON_ID"),
		}

		rows[i] = v
	}
	return StandbyPromotionSCNResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadStandbyPromotionSCN completes one bounded observation on an admitted reader.
func ReadStandbyPromotionSCN(ctx context.Context, reader Reader, request RecoveryIdentityRequest) (StandbyPromotionSCNResult, error) {
	p, err := standbyPromotionSCNPlan(request.OracleVersion)
	if err != nil {
		return StandbyPromotionSCNResult{}, err
	}
	result, err := readRecoveryIdentity(ctx, reader, request, p)
	if err != nil {
		return StandbyPromotionSCNResult{}, err
	}
	facts, err := decodeStandbyPromotionSCN(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return StandbyPromotionSCNResult{}, &Error{p.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// StandbyPromotionSCNRows transfers the caller-native cursor to its owner.
func StandbyPromotionSCNRows(ctx context.Context, q rowset.Queryer, version string) (*sql.Rows, error) {
	p, err := standbyPromotionSCNPlan(version)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// StandbyPromotionSCNJSON reads one bounded complete dataset on a separate cursor.
func StandbyPromotionSCNJSON(ctx context.Context, q rowset.Queryer, version string, limits rowset.Limits) (json.RawMessage, error) {
	p, err := standbyPromotionSCNPlan(version)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits)
}
