package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestExecutionStatementPreservesProjection(t *testing.T) {
	t.Parallel()
	const want = "SELECT d.name AS database_name, TO_CHAR(d.dbid,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS dbid, i.version_full AS oracle_version, TO_CHAR(i.instance_number,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS instance_id, SYS_CONTEXT('USERENV','CON_ID') AS container_id, SYS_CONTEXT('USERENV','SERVICE_NAME') AS service_name, TO_CHAR(i.startup_time,'YYYY-MM-DD HH24:MI:SS') AS startup_key FROM v$database d CROSS JOIN v$instance i"
	if got := identity.ExecutionStatement(); got != want {
		t.Fatalf("SQL = %q, want %q", got, want)
	}
}

func executionResult() session.Result {
	return session.Result{
		Columns: []string{"DATABASE_NAME", "DBID", "ORACLE_VERSION", "INSTANCE_ID", "CONTAINER_ID", "SERVICE_NAME", "STARTUP_KEY"},
		Rows: [][]session.Value{{
			{Text: " DB_α "}, {Text: "9007199254740993"}, {Text: "future.release"},
			{Text: "001"}, {Text: "0"}, {Text: " svc\t"}, {Text: "2026-10-25 01:30:00"},
		}},
	}
}

func expectedExecution() identity.Execution {
	available := func(value string) identity.Text { return identity.Text{State: identity.Available, Value: value} }
	return identity.Execution{
		Database: available(" DB_α "), DBID: available("9007199254740993"), Version: available("future.release"),
		Instance: available("001"), Container: available("0"), Service: available(" svc\t"), Startup: available("2026-10-25 01:30:00"),
		InstanceID: 1, ContainerID: 0,
	}
}

func TestDecodeExecutionPreservesFacts(t *testing.T) {
	t.Parallel()
	got, err := identity.DecodeExecution(executionResult())
	if want := expectedExecution(); err != nil || got != want {
		t.Fatalf("decode = %+v, %v; want %+v, nil", got, err, want)
	}
	for _, tt := range []struct {
		name  string
		index int
		field func(*identity.Execution) *identity.Text
	}{
		{"database", 0, func(e *identity.Execution) *identity.Text { return &e.Database }},
		{"dbid", 1, func(e *identity.Execution) *identity.Text { return &e.DBID }},
		{"version", 2, func(e *identity.Execution) *identity.Text { return &e.Version }},
		{"instance", 3, func(e *identity.Execution) *identity.Text { return &e.Instance }},
		{"container", 4, func(e *identity.Execution) *identity.Text { return &e.Container }},
		{"service", 5, func(e *identity.Execution) *identity.Text { return &e.Service }},
		{"startup", 6, func(e *identity.Execution) *identity.Text { return &e.Startup }},
	} {
		t.Run(tt.name+" NULL", func(t *testing.T) {
			r := executionResult()
			r.Rows[0][tt.index] = session.Value{Null: true, Text: "ignored"}
			want := expectedExecution()
			*tt.field(&want) = identity.Text{State: identity.Null}
			if tt.index == 3 {
				want.InstanceID = 0
			}
			got, err := identity.DecodeExecution(r)
			if err != nil || got != want {
				t.Fatalf("decode = %+v, %v; want %+v, nil", got, err, want)
			}
		})
	}
}

func TestDecodeExecutionAbsenceAndEmptyText(t *testing.T) {
	t.Parallel()
	r := executionResult()
	r.Rows = nil
	want := identity.Execution{
		Database: identity.Text{State: identity.NoRows}, DBID: identity.Text{State: identity.NoRows}, Version: identity.Text{State: identity.NoRows},
		Instance: identity.Text{State: identity.NoRows}, Container: identity.Text{State: identity.NoRows},
		Service: identity.Text{State: identity.NoRows}, Startup: identity.Text{State: identity.NoRows},
	}
	got, err := identity.DecodeExecution(r)
	if err != nil || got != want {
		t.Fatalf("no rows = %+v, %v; want %+v, nil", got, err, want)
	}
	r = executionResult()
	for _, i := range []int{0, 1, 2, 5, 6} {
		r.Rows[0][i] = session.Value{}
	}
	want = expectedExecution()
	want.Database, want.DBID, want.Version, want.Service, want.Startup = identity.Text{State: identity.Available}, identity.Text{State: identity.Available}, identity.Text{State: identity.Available}, identity.Text{State: identity.Available}, identity.Text{State: identity.Available}
	got, err = identity.DecodeExecution(r)
	if err != nil || got != want {
		t.Fatalf("empty text = %+v, %v; want %+v, nil", got, err, want)
	}
}

func TestDecodeExecutionNumericIDs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		instance  string
		container string
		wantID    int
		wantConID int
		invalid   bool
	}{
		{"positive container", "2", "3", 2, 3, false},
		{"source integer spelling", "+001", "+000", 1, 0, false},
		{"zero instance", "0", "3", 0, 3, true},
		{"negative instance", "-1", "3", 0, 3, true},
		{"empty instance", "", "3", 0, 3, true},
		{"fractional instance", "1.0", "3", 0, 3, true},
		{"padded instance", " 1", "3", 0, 3, true},
		{"instance overflow", strings.Repeat("9", 100), "3", 0, 3, true},
		{"negative container", "1", "-1", 1, 0, true},
		{"empty container", "1", "", 1, 0, true},
		{"fractional container", "1", "0.0", 1, 0, true},
		{"exponent container", "1", "1e1", 1, 0, true},
		{"container overflow", "1", strings.Repeat("9", 100), 1, 0, true},
		{"both invalid", "bad instance", "bad container", 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := executionResult()
			r.Rows[0][3].Text, r.Rows[0][4].Text = tt.instance, tt.container
			want := expectedExecution()
			want.Instance.Value, want.Container.Value = tt.instance, tt.container
			want.InstanceID, want.ContainerID = tt.wantID, tt.wantConID
			got, err := identity.DecodeExecution(r)
			if got != want || (tt.invalid && !errors.Is(err, identity.ErrInvalidEvidence)) || (!tt.invalid && err != nil) {
				t.Fatalf("decode = %+v, %v; want %+v, invalid=%t", got, err, want, tt.invalid)
			}
			if err != nil && (strings.Contains(err.Error(), tt.instance) && len(tt.instance) > 10 || strings.Contains(err.Error(), tt.container) && len(tt.container) > 10) {
				t.Fatal("error exposes source values")
			}
		})
	}
}

func TestDecodeExecutionMetadataAndShape(t *testing.T) {
	t.Parallel()
	r := executionResult()
	for i, column := range r.Columns {
		r.Columns[i] = strings.ToLower(column)
	}
	r.Columns[0], r.Columns[6] = r.Columns[6], r.Columns[0]
	r.Rows[0][0], r.Rows[0][6] = r.Rows[0][6], r.Rows[0][0]
	got, err := identity.DecodeExecution(r)
	if err != nil || got != expectedExecution() {
		t.Fatalf("reordered metadata = %+v, %v", got, err)
	}
	for _, tt := range []struct {
		name   string
		change func(*session.Result)
	}{
		{"missing column", func(r *session.Result) { r.Columns = r.Columns[:6] }},
		{"extra column", func(r *session.Result) { r.Columns = append(r.Columns, "OTHER") }},
		{"wrong column", func(r *session.Result) { r.Columns[0] = "NAME" }},
		{"duplicate column", func(r *session.Result) { r.Columns[0] = r.Columns[1] }},
		{"missing value", func(r *session.Result) { r.Rows[0] = r.Rows[0][:6] }},
		{"extra value", func(r *session.Result) { r.Rows[0] = append(r.Rows[0], session.Value{}) }},
		{"multiple rows", func(r *session.Result) { r.Rows = append(r.Rows, r.Rows[0]) }},
		{"wrong empty source", func(r *session.Result) { r.Columns[0] = "NAME"; r.Rows = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := executionResult()
			tt.change(&r)
			got, err := identity.DecodeExecution(r)
			if got != (identity.Execution{}) || !errors.Is(err, identity.ErrInvalidEvidence) {
				t.Fatalf("malformed decode = %+v, %v; want zero facts, ErrInvalidEvidence", got, err)
			}
		})
	}
}
