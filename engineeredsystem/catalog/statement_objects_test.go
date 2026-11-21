package catalog

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/diagnostic"
)

type statementObjectReporter interface {
	StatementObjects(error) ([]diagnostic.ObjectStatus, error)
}

func TestStatementObjectsUsesReviewedSelectedRelation(t *testing.T) {
	const major = 23
	cells := map[string]string{"CELL_HASHVAL": "NUMBER", "CELL_PATH": "VARCHAR2"}
	globalCells := map[string]string{"INST_ID": "NUMBER", "CELL_HASHVAL": "NUMBER", "CELL_PATH": "VARCHAR2"}
	config := map[string]string{"CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "CLOB"}
	globalConfig := map[string]string{"INST_ID": "NUMBER", "CELLNAME": "VARCHAR2", "CONFTYPE": "VARCHAR2", "CONFVAL": "CLOB"}
	alertXML := map[string]string{"CELL_NAME": "VARCHAR2", "ALERT_VALUE": "CLOB"}
	alertScalar := map[string]string{"INST_ID": "NUMBER", "CELL_NAME": "VARCHAR2", "CELL_HASH": "NUMBER", "BEGIN_TIME": "DATE", "SEQ_NO": "NUMBER", "MESSAGE": "VARCHAR2", "STATEFUL": "VARCHAR2", "SEVERITY": "VARCHAR2"}
	cases := []struct {
		name   string
		report statementObjectReporter
		schema string
		object string
	}{
		{"column metadata", MetadataOptions{OracleMajor: major}, "SYS", "ALL_TAB_COLUMNS"},
		{"global cells", CellsOptions{OracleMajor: major, Schema: Schema{"GV_$CELL": globalCells, "V_$CELL": cells}}, "SYS", "GV_$CELL"},
		{"local cells after capability fallback", CellsOptions{OracleMajor: major, Schema: Schema{"GV_$CELL": cells, "V_$CELL": cells}}, "SYS", "V_$CELL"},
		{"bounded cell with type", ConnectedCellsOptions{OracleMajor: major, Limit: 5, IncludeType: true}, "", "V$CELL"},
		{"bounded cell without type", ConnectedCellsOptions{OracleMajor: major, Limit: 5, CellTypeKnownAbsent: true}, "", "V$CELL"},
		{"visible cell count", VisibleCellCountOptions{OracleMajor: major}, "", "GV$CELL"},
		{"global config", ConfigurationOptions{OracleMajor: major, Schema: Schema{"GV_$CELL_CONFIG": globalConfig, "V_$CELL_CONFIG_INFO": config}}, "SYS", "GV_$CELL_CONFIG"},
		{"local config", ConfigurationOptions{OracleMajor: major, Schema: Schema{"V_$CELL_CONFIG": config}}, "SYS", "V_$CELL_CONFIG"},
		{"global config info", ConfigurationOptions{OracleMajor: major, Info: true, Schema: Schema{"GV_$CELL_CONFIG_INFO": globalConfig}}, "SYS", "GV_$CELL_CONFIG_INFO"},
		{"local config info after capability fallback", ConfigurationOptions{OracleMajor: major, Info: true, Schema: Schema{"GV_$CELL_CONFIG_INFO": config, "V_$CELL_CONFIG_INFO": config}}, "SYS", "V_$CELL_CONFIG_INFO"},
		{"global scalar alert", AlertsOptions{OracleMajor: major, Schema: Schema{"GV_$CELL_OPEN_ALERTS": alertScalar}}, "SYS", "GV_$CELL_OPEN_ALERTS"},
		{"local XML alert after capability fallback", AlertsOptions{OracleMajor: major, Schema: Schema{"GV_$CELL_OPEN_ALERTS": alertXML, "V_$CELL_OPEN_ALERTS": alertXML}}, "SYS", "V_$CELL_OPEN_ALERTS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := diagnostic.ObjectStatus{ObjectRef: diagnostic.ObjectRef{Schema: tc.schema, Name: tc.object}}
			if tc.schema != "" {
				want.Kind = "view"
			}
			report, err := tc.report.StatementObjects(nil)
			if err != nil || !reflect.DeepEqual(report, []diagnostic.ObjectStatus{want}) {
				t.Fatal(report, err, want)
			}
			report[0].Name = "caller modification"
			owner := tc.schema
			if owner == "" {
				owner = "SYS"
			}
			cause := fmt.Errorf("ORA-00942: table or view %s.%s does not exist", owner, tc.object)
			if tc.schema != "" {
				want.ProblemCode = "ORA-00942"
			}
			report, err = tc.report.StatementObjects(fmt.Errorf("execution failed: %w", cause))
			if err != nil || !reflect.DeepEqual(report, []diagnostic.ObjectStatus{want}) {
				t.Fatal(report, err, want)
			}
		})
	}
}

func TestStatementObjectsLeavesUnprovenErrorsUnmarked(t *testing.T) {
	options := MetadataOptions{OracleMajor: 23}
	target := errors.New("ORA-00942: table or view SYS.ALL_TAB_COLUMNS does not exist")
	for _, cause := range []error{
		errors.New("ORA-00942: table or view does not exist"),
		errors.New("ORA-01031: insufficient privileges"),
		errors.New("ORA-00942: table or view SYS.SOME_OTHER_VIEW does not exist"),
		errors.Join(target, errors.New("ORA-02063: preceding line from REMOTE")),
		errors.Join(target, errors.New("ORA-00942: table or view does not exist")),
	} {
		report, err := options.StatementObjects(cause)
		if err != nil || len(report) != 1 || report[0].Name != "ALL_TAB_COLUMNS" || report[0].ProblemCode != "" {
			t.Fatal(report, err)
		}
	}
	report, err := options.StatementObjects(errors.New(strings.Repeat("x", diagnostic.MaxBytes+1)))
	if !errors.Is(err, diagnostic.ErrCapacity) || len(report) != 1 || report[0].Name != "ALL_TAB_COLUMNS" || report[0].ProblemCode != "" {
		t.Fatal(report, err)
	}
	report, err = (CellsOptions{OracleMajor: 23}).StatementObjects(target)
	if report != nil || !errors.Is(err, ErrUnsupportedSource) {
		t.Fatal(report, err)
	}
	report, err = (MetadataOptions{OracleMajor: 9}).StatementObjects(target)
	if report != nil || !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatal(report, err)
	}
}

func TestStatementObjectsConsumesOriginalRowsError(t *testing.T) {
	options := MetadataOptions{OracleMajor: 23}
	cause := errors.New("ORA-00942: table or view SYS.ALL_TAB_COLUMNS does not exist")
	db, state := testsql.New(t, testsql.Step{SQL: MetadataSQL, QueryErr: cause})
	cursor, executionErr := MetadataRows(t.Context(), db, options)
	var domain *DatasetError
	if cursor != nil || !errors.Is(executionErr, cause) || !errors.As(executionErr, &domain) || domain.Source != "SYS.ALL_TAB_COLUMNS" {
		t.Fatal(cursor, executionErr)
	}
	report, err := options.StatementObjects(executionErr)
	if err != nil || len(report) != 1 || report[0].Name != "ALL_TAB_COLUMNS" || report[0].ProblemCode != "ORA-00942" ||
		len(state.Calls()) != 1 || !errors.Is(executionErr, cause) {
		t.Fatal(report, err, executionErr)
	}
}
