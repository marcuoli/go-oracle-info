package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

// Service retains source spelling, exact integer text and selected SQL NULL.
// CreationDate is source-local civil time without an inferred timezone.
// MinCardinality, MaxCardinality and Enabled are documented reserved fields;
// their raw values do not establish service availability. Edition is nil when
// unselected on 11g, or nonnil Null when selected SQL NULL.
type Service struct {
	Name              session.Value
	NetworkName       session.Value
	CreationDate      session.Value
	CreationDateHash  session.Value
	FailoverMethod    session.Value
	FailoverType      session.Value
	FailoverRetries   session.Value
	FailoverDelay     session.Value
	MinCardinality    session.Value
	MaxCardinality    session.Value
	Goal              session.Value
	DTP               session.Value
	Enabled           session.Value
	AQHANotifications session.Value
	CLBGoal           session.Value
	Edition           *session.Value
}

// Result retains complete source order and duplicates, with source visibility.
// A successful empty inventory says nothing about unadmitted sources.
type Result struct {
	Provenance Provenance
	Rows       []Service
}

// Statement selects DBA_SERVICES. The caller admits privileges, container and open state.
// Major 11 omits EDITION unless the caller explicitly admits its 11.2.0.2+ column
// with includeEdition=true; 12+ always selects it. No reachability is inferred.
func Statement(major int, includeEdition ...bool) (string, error) {
	if len(includeEdition) > 1 {
		return "", ErrInvalidRequest
	}
	if major < 11 {
		return "", ErrUnsupportedVersion
	}
	statement := `SELECT NAME AS NAME, NETWORK_NAME AS NETWORK_NAME, TO_CHAR(CREATION_DATE,'YYYY-MM-DD"T"HH24:MI:SS') AS CREATION_DATE, TO_CHAR(CREATION_DATE_HASH,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS CREATION_DATE_HASH, FAILOVER_METHOD AS FAILOVER_METHOD, FAILOVER_TYPE AS FAILOVER_TYPE, TO_CHAR(FAILOVER_RETRIES,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS FAILOVER_RETRIES, TO_CHAR(FAILOVER_DELAY,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS FAILOVER_DELAY, TO_CHAR(MIN_CARDINALITY,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS MIN_CARDINALITY, TO_CHAR(MAX_CARDINALITY,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS MAX_CARDINALITY, GOAL AS GOAL, DTP AS DTP, ENABLED AS ENABLED, AQ_HA_NOTIFICATIONS AS AQ_HA_NOTIFICATIONS, CLB_GOAL AS CLB_GOAL`
	if serviceEdition(major, includeEdition) {
		statement += ", EDITION AS EDITION"
	}
	return statement + " FROM DBA_SERVICES ORDER BY NAME", nil
}

func serviceSchema(major int, includeEdition ...bool) []rowset.Column {
	schema := []rowset.Column{
		{Name: "NAME", Kind: rowset.Text},
		{Name: "NETWORK_NAME", Kind: rowset.Text},
		{Name: "CREATION_DATE", Kind: rowset.Text},
		{Name: "CREATION_DATE_HASH", Kind: rowset.Integer},
		{Name: "FAILOVER_METHOD", Kind: rowset.Text},
		{Name: "FAILOVER_TYPE", Kind: rowset.Text},
		{Name: "FAILOVER_RETRIES", Kind: rowset.Integer},
		{Name: "FAILOVER_DELAY", Kind: rowset.Integer},
		{Name: "MIN_CARDINALITY", Kind: rowset.Integer},
		{Name: "MAX_CARDINALITY", Kind: rowset.Integer},
		{Name: "GOAL", Kind: rowset.Text},
		{Name: "DTP", Kind: rowset.Text},
		{Name: "ENABLED", Kind: rowset.Text},
		{Name: "AQ_HA_NOTIFICATIONS", Kind: rowset.Text},
		{Name: "CLB_GOAL", Kind: rowset.Text},
	}

	if serviceEdition(major, includeEdition) {
		schema = append(schema, rowset.Column{Name: "EDITION", Kind: rowset.Text})
	}
	return schema
}

func serviceProvenance(major int, includeEdition ...bool) Provenance {
	p := Provenance{Source: "DBA_SERVICES", Scope: "current_container_dictionary", OracleMajor: major}

	if serviceEdition(major, includeEdition) {
		p.OptionalColumns = []string{"EDITION"}
	}
	return p
}

// Decode validates every row before publishing anything, including malformed late
// rows. A pure decoder does not establish transport completion or admission.
func Decode(result session.Result, major int, includeEdition ...bool) (Result, error) {
	if _, err := Statement(major, includeEdition...); err != nil {
		return Result{}, err
	}
	index, err := factColumns(result, serviceSchema(major, includeEdition...))
	if err != nil {
		return Result{}, err
	}

	facts := make([]Service, 0, len(result.Rows))
	for _, row := range result.Rows {
		fact := Service{
			Name:              row[index["NAME"]],
			NetworkName:       row[index["NETWORK_NAME"]],
			CreationDate:      row[index["CREATION_DATE"]],
			CreationDateHash:  row[index["CREATION_DATE_HASH"]],
			FailoverMethod:    row[index["FAILOVER_METHOD"]],
			FailoverType:      row[index["FAILOVER_TYPE"]],
			FailoverRetries:   row[index["FAILOVER_RETRIES"]],
			FailoverDelay:     row[index["FAILOVER_DELAY"]],
			MinCardinality:    row[index["MIN_CARDINALITY"]],
			MaxCardinality:    row[index["MAX_CARDINALITY"]],
			Goal:              row[index["GOAL"]],
			DTP:               row[index["DTP"]],
			Enabled:           row[index["ENABLED"]],
			AQHANotifications: row[index["AQ_HA_NOTIFICATIONS"]],
			CLBGoal:           row[index["CLB_GOAL"]],
		}

		if serviceEdition(major, includeEdition) {
			value := row[index["EDITION"]]
			fact.Edition = &value
		}
		facts = append(facts, fact)
	}

	return Result{Provenance: serviceProvenance(major, includeEdition...), Rows: facts}, nil
}

// Read executes once on the already-owned reader and publishes no failed prefix.
// It owns no connection, retry, grant or container change.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return Result{}, err
	}
	statement, err := Statement(request.OracleMajor, request.IncludeEdition)
	if err != nil {
		return Result{}, err
	}
	if err = factReadError(ctx, nil); err != nil {
		return Result{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
	if err = factReadError(ctx, err); err != nil {
		return Result{}, err
	}
	facts, err := Decode(result, request.OracleMajor, request.IncludeEdition)
	if err = factReadError(ctx, err); err != nil {
		return Result{}, err
	}
	return facts, nil
}

// Rows returns the actual caller-native cursor unchanged. The caller owns its
// Next/Scan/Err/Close lifecycle and budgets; JSON independently owns its cursor.
func Rows(ctx context.Context, q rowset.Queryer, major int, includeEdition ...bool) (*sql.Rows, error) {
	statement, err := Statement(major, includeEdition...)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// JSON independently consumes/finalizes a cursor. It returns no JSON on terminal,
// scan, close, cancellation or capacity failure; the budget includes provenance.
func JSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits, includeEdition ...bool) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	provenance := serviceProvenance(major, includeEdition...)
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: provenance}
	metadata, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(metadata) - 4
	if budget.MaxBytes < 1 {
		return nil, rowset.ErrCapacity
	}

	cursor, err := Rows(ctx, q, major, includeEdition...)
	if err != nil {
		return nil, err
	}
	envelope.Data, err = rowset.ReadJSON(ctx, cursor, serviceSchema(major, includeEdition...), budget)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = factReadError(ctx, err); err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// Oracle documents EDITION from 11.2.0.2. Major 11 alone cannot admit it.
func serviceEdition(major int, include []bool) bool {
	return major >= 12 || (len(include) == 1 && include[0])
}
