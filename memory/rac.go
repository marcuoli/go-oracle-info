package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"

	numtext "github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// MemberValue retains an effective member setting in raw bytes before rounding.
type MemberValue struct {
	InstanceID        int
	SourceContainerID int
	Parameter         string
	ValueBytes        string
	Origin            Origin
}

// Total retains independently valid member evidence even when no total is safe.
type Total struct {
	State    State
	ValueGiB string
	Values   []MemberValue
}

// RACObservation separates proven member totals from connected-instance values.
type RACObservation struct {
	Members []int
	SGA     Total
	PGA     Total
}

// RACStatement enumerates member rows before parameter selection. PDB membership
// covers only instances where that PDB is open. Callers must exclude managed ADB
// backing-cluster scope using their admitted capabilities before requesting RAC.
func RACStatement(o Options) (string, error) {
	if err := validateOptions(o); err != nil {
		return "", err
	}
	if o.PDB.EffectiveADB {
		return "", ErrUnsupportedScope
	}
	source := "0"
	target := "0"
	table := "GV$INSTANCE"
	condition := ""
	names := "'sga_max_size','pga_aggregate_target'"
	where := ""
	if o.OracleMajor >= 12 {
		source = "NVL(P.CON_ID,0)"
		target = "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))"
		condition = " AND P.CON_ID IN (0,1)"
	}
	if o.Scope == ConnectedPDB || o.Scope == DiscoveredPDB {
		table = "GV$PDBS"
		names = "'sga_target','sga_max_size','pga_aggregate_target'"
		if o.Scope == DiscoveredPDB {
			target = ":target_container_id"
		}
		condition = " AND P.CON_ID IN (" + target + ")"
		if o.PDB.AllowInherited {
			condition = " AND P.CON_ID IN (" + target + ",0,1)"
		}
		where = " WHERE I.CON_ID=" + target + " AND I.OPEN_MODE IN ('READ WRITE','READ ONLY')"
	}
	return "SELECT P.NAME AS PARAMETER_NAME, P.VALUE AS PARAMETER_VALUE, " +
		numeric("I.INST_ID") + " AS INSTANCE_ID, " + numeric(source) + " AS SOURCE_CONTAINER_ID, " + numeric(target) + " AS TARGET_CONTAINER_ID\n" +
		"FROM " + table + " I LEFT JOIN GV$SYSTEM_PARAMETER P ON P.INST_ID=I.INST_ID AND P.NAME IN (" + names + ")" + condition + where, nil
}

type parameterRow struct {
	name             string
	value            session.Value
	instance, source int
}

func schema(result session.Result, names []string) (map[string]int, error) {
	if len(result.Columns) != len(names) {
		return nil, ErrInvalidEvidence
	}
	index := make(map[string]int, len(names))
	for i, name := range result.Columns {
		name = strings.ToLower(name)
		if _, ok := index[name]; name == "" || ok {
			return nil, ErrInvalidEvidence
		}
		index[name] = i
	}
	for _, name := range names {
		if _, ok := index[name]; !ok {
			return nil, ErrInvalidEvidence
		}
	}
	for _, row := range result.Rows {
		if len(row) != len(names) {
			return nil, ErrInvalidEvidence
		}
	}
	return index, nil
}

// DecodeRAC validates membership and resolves each metric per member. A missing,
// NULL, duplicate or invalid selected member prevents only that metric's total;
// valid member evidence and the other independently validated total remain.
func DecodeRAC(result session.Result, o Options) (RACObservation, error) {
	report := RACObservation{SGA: Total{State: Unavailable}, PGA: Total{State: Unavailable}}
	if err := validateOptions(o); err != nil {
		return report, err
	}
	if o.PDB.EffectiveADB {
		return report, ErrUnsupportedScope
	}
	names := []string{"parameter_name", "parameter_value", "instance_id", "source_container_id", "target_container_id"}
	index, err := schema(result, names)
	if err != nil {
		return report, err
	}
	if len(result.Rows) > 4096 {
		return report, fmt.Errorf("%w: RAC source exceeds 4096 rows; no prefix admitted", ErrInvalidEvidence)
	}
	rows := make([]parameterRow, 0, len(result.Rows))
	members := map[int]bool{}
	container := -1
	for _, row := range result.Rows {
		instance, ok := integerCell(row[index["instance_id"]])
		if !ok || instance <= 0 {
			return report, ErrInvalidEvidence
		}
		target, ok := integerCell(row[index["target_container_id"]])
		if !ok {
			return report, ErrInvalidEvidence
		}
		if (o.Scope == NonCDB && target != 0) || (o.Scope == CDBRoot && target != 1) || ((o.Scope == ConnectedPDB || o.Scope == DiscoveredPDB) && target <= 2) {
			return report, ErrInvalidEvidence
		}
		if o.TargetContainerID != 0 && target != o.TargetContainerID {
			return report, ErrInvalidEvidence
		}
		if container >= 0 && container != target {
			return report, ErrInvalidEvidence
		}
		container = target
		source, ok := integerCell(row[index["source_container_id"]])
		if !ok || (source != 0 && source != 1 && source != target) {
			return report, ErrInvalidEvidence
		}
		if o.Scope == NonCDB && source != 0 {
			return report, ErrInvalidEvidence
		}
		members[instance] = true
		name := row[index["parameter_name"]]
		if name.Null {
			if !row[index["parameter_value"]].Null {
				return report, ErrInvalidEvidence
			}
			continue
		}
		if name.Text != "sga_target" && name.Text != "sga_max_size" && name.Text != "pga_aggregate_target" {
			return report, ErrInvalidEvidence
		}
		if target > 2 && !o.PDB.AllowInherited && source != target {
			return report, ErrInvalidEvidence
		}
		rows = append(rows, parameterRow{name: name.Text, value: row[index["parameter_value"]], instance: instance, source: source})
	}
	for member := range members {
		report.Members = append(report.Members, member)
	}
	slices.Sort(report.Members)
	if len(report.Members) == 0 {
		report.SGA.State, report.PGA.State = NoRows, NoRows
		return report, nil
	}
	var failures []error
	for _, metric := range []struct {
		parameter Parameter
		total     *Total
	}{{SGA, &report.SGA}, {PGA, &report.PGA}} {
		metric.total.State = Available
		sum := new(big.Int)
		for _, instance := range report.Members {
			value, err := effectiveMember(rows, instance, container, metric.parameter, o.PDB.AllowInherited)
			if err != nil {
				metric.total.State = Unavailable
				failures = append(failures, fmt.Errorf("configured %s instance %d: %w", metric.parameter, instance, err))
				continue
			}
			metric.total.Values = append(metric.total.Values, value)
			number, _ := new(big.Int).SetString(value.ValueBytes, 10)
			sum.Add(sum, number)
		}
		if metric.total.State == Available {
			metric.total.ValueGiB = formatGiB(sum)
		}
	}
	return report, errors.Join(failures...)
}

func effectiveMember(rows []parameterRow, instance, container int, parameter Parameter, allowInherited bool) (MemberValue, error) {
	levels := []int{0, 1}
	if container > 2 {
		levels = []int{container}
		if allowInherited {
			levels = append(levels, 0, 1)
		}
	}
	name := "pga_aggregate_target"
	if parameter == SGA {
		name = "sga_max_size"
	}
	for _, level := range levels {
		selected := name
		if container > 2 && level == container && parameter == SGA {
			selected = "sga_target"
		}
		var matches []parameterRow
		for _, row := range rows {
			if row.instance == instance && row.source == level && row.name == selected {
				matches = append(matches, row)
			}
		}
		if len(matches) == 0 {
			continue
		}
		if len(matches) != 1 {
			return MemberValue{}, fmt.Errorf("%w: duplicate effective rows", ErrInvalidEvidence)
		}
		cell := matches[0].value
		if cell.Null {
			return MemberValue{}, fmt.Errorf("%w: NULL effective parameter", ErrInvalidEvidence)
		}
		if len(cell.Text) > 128 || !numtext.IsUnsignedInteger(cell.Text) {
			return MemberValue{}, fmt.Errorf("%w: invalid byte value", ErrInvalidEvidence)
		}
		number, ok := new(big.Int).SetString(cell.Text, 10)
		if !ok {
			return MemberValue{}, ErrInvalidEvidence
		}
		if selected == "sga_target" && number.Sign() == 0 {
			continue
		}
		origin := InstanceParameter
		if container > 2 {
			origin = ContainerParameter
			if level != container {
				origin = InheritedInstance
			}
		}
		return MemberValue{InstanceID: instance, SourceContainerID: level, Parameter: selected, ValueBytes: strings.Clone(cell.Text), Origin: origin}, nil
	}
	return MemberValue{}, fmt.Errorf("%w: missing effective parameter", ErrInvalidEvidence)
}

func formatGiB(bytes *big.Int) string {
	// Convert the exact byte total, rounding once after aggregation.
	gib := new(big.Rat).SetFrac(bytes, big.NewInt(1<<30))
	decimal := gib.FloatString(3)
	return strings.TrimSuffix(strings.TrimRight(decimal, "0"), ".")
}

// CollectRAC reads a previously admitted non-ADB RAC scope through the owned
// reader. It does not discover or admit a backing cluster, retry errors or change
// sessions. The reader must retain complete results and enforce its row budget.
func CollectRAC(ctx context.Context, reader Reader, options Options, ref reference.Reference) (RACObservation, error) {
	unavailable := RACObservation{SGA: Total{State: Unavailable}, PGA: Total{State: Unavailable}}
	if reader == nil || ref.Namespace() == "" {
		return unavailable, errors.New("RAC reader and execution reference are required")
	}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	text, err := RACStatement(options)
	if err != nil {
		return unavailable, err
	}
	var args []any
	if options.Scope == DiscoveredPDB {
		args = []any{sql.Named("target_container_id", options.TargetContainerID)}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: text, Args: args})
	missing := onlyNoRows(err)
	if missing {
		err = nil
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return unavailable, &ReadError{cause: err}
	}
	if missing {
		return RACObservation{SGA: Total{State: NoRows}, PGA: Total{State: NoRows}}, nil
	}
	observation, err := DecodeRAC(result, options)
	if cancelled := ctx.Err(); cancelled != nil {
		return unavailable, errors.Join(err, cancelled)
	}
	return observation, err
}

func racPlan(options Options) (datasetSelection, error) {
	if err := datasetMajor(options.OracleMajor); err != nil {
		return datasetSelection{}, err
	}
	text, err := RACStatement(options)
	if err != nil {
		return datasetSelection{}, err
	}
	source := "GV$INSTANCE+GV$SYSTEM_PARAMETER"
	if options.Scope == ConnectedPDB || options.Scope == DiscoveredPDB {
		source = "GV$PDBS+GV$SYSTEM_PARAMETER"
	}
	names := []string{"PARAMETER_NAME", "PARAMETER_VALUE", "INSTANCE_ID", "SOURCE_CONTAINER_ID", "TARGET_CONTAINER_ID"}
	return memorySelection(text, source, "", options, names), nil
}

// ReadRAC retains CollectRAC's independently valid metric evidence alongside
// errors. This is aggregate evidence, never a failed cursor prefix or new retry.
func ReadRAC(ctx context.Context, reader Reader, options Options, ref reference.Reference) (RACObservation, error) {
	unavailable := RACObservation{SGA: Total{State: Unavailable}, PGA: Total{State: Unavailable}}
	p, err := racPlan(options)
	if err != nil {
		return unavailable, err
	}
	if !validDatasetReader(reader) || ref.Validate() != nil {
		return unavailable, ErrInvalidRequest
	}
	if err = datasetReadError(ctx, p, nil); err != nil {
		return unavailable, err
	}
	result, err := reader.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: ref})
	missing := onlyNoRows(err)
	if missing {
		err = nil
	}
	if err = datasetReadError(ctx, p, err); err != nil {
		return unavailable, err
	}
	if missing {
		return RACObservation{SGA: Total{State: NoRows}, PGA: Total{State: NoRows}}, nil
	}
	// Validate the entire transport before the legacy aggregate decoder can skip
	// an absent metric name. A contradictory NULL scalar is not partial evidence.
	// Valid independently available metrics still survive domain decoding errors.
	if _, err = datasetColumns(result, p); err != nil {
		return unavailable, err
	}
	observation, err := DecodeRAC(result, options)
	if ctx.Err() != nil {
		return unavailable, datasetReadError(ctx, p, err)
	}
	return observation, err
}

// RACRows returns the actual native RACStatement cursor, not a fabricated total.
func RACRows(ctx context.Context, q rowset.Queryer, options Options) (*sql.Rows, error) {
	p, err := racPlan(options)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, p.sql, p.args...)
}

// RACJSON preserves complete source rows; it does not infer physical membership
// or certify both totals. Domain/transport/cancellation/budget failures return nil.
func RACJSON(ctx context.Context, q rowset.Queryer, options Options, limits rowset.Limits) (json.RawMessage, error) {
	p, err := racPlan(options)
	if err != nil {
		return nil, err
	}
	return readDatasetJSON(ctx, q, p, limits, func(r session.Result) error { _, err := DecodeRAC(r, options); return err })
}

// RACRequest adds the caller's SQL reference to one admitted RAC selection.
type RACRequest struct {
	Options   Options
	Reference reference.Reference
}

// ReadRACWithOptions reads RAC memory using a single caller request.
func ReadRACWithOptions(ctx context.Context, reader Reader, request RACRequest) (RACObservation, error) {
	return ReadRAC(ctx, reader, request.Options, request.Reference)
}
