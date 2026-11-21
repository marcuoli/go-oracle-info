package registry

import (
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func TestComponentsJSONPreservesLegacyEnvelopeAndBudget(t *testing.T) {
	options := ComponentsOptions{OracleMajor: 19}
	statement, err := ComponentsStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{
		SQL:     statement,
		Columns: []string{"COMP_ID", "COMP_NAME", "VERSION", "VERSION_FULL", "STATUS", "MODIFIED"},
		Values:  [][]driver.Value{{"CATALOG", "Dictionary", "19.0.0.0.0", nil, "  Valid  ", "raw date"}},
	}
	const want = "{" +
		"\"provenance\":{\"source\":\"DBA_REGISTRY\",\"scope\":\"current_container_dictionary\",\"oracle_major\":19}," +
		"\"data\":{\"columns\":[{\"name\":\"COMP_ID\",\"kind\":\"text\"},{\"name\":\"COMP_NAME\",\"kind\":\"text\"}," +
		"{\"name\":\"VERSION\",\"kind\":\"text\"},{\"name\":\"VERSION_FULL\",\"kind\":\"text\"}," +
		"{\"name\":\"STATUS\",\"kind\":\"text\"},{\"name\":\"MODIFIED\",\"kind\":\"text\"}]," +
		"\"rows\":[[\"CATALOG\",\"Dictionary\",\"19.0.0.0.0\",null,\"  Valid  \",\"raw date\"]]}}"
	limits := rowset.Limits{MaxBytes: len(want)}
	db, state := testsql.New(t, step, step)
	legacy, err := JSON(t.Context(), db, options.OracleMajor, limits)
	if err != nil || string(legacy) != want {
		t.Fatalf("legacy JSON = %s, err = %v", legacy, err)
	}
	current, err := ComponentsJSON(t.Context(), db, options, limits)
	if err != nil || string(current) != want || state.Closed() != 2 {
		t.Fatalf("named JSON = %s, err = %v, closed = %d", current, err, state.Closed())
	}

	db, state = testsql.New(t, step)
	limits.MaxBytes--
	payload, err := ComponentsJSON(t.Context(), db, options, limits)
	var transport *rowset.Error
	if payload != nil || !errors.Is(err, rowset.ErrCapacity) ||
		!errors.As(err, &transport) || transport.Operation != "budget" || state.Closed() != 1 {
		t.Fatalf("short budget JSON = %s, err = %v, closed = %d", payload, err, state.Closed())
	}

	db, state = testsql.New(t)
	payload, err = ComponentsJSON(t.Context(), db, options, rowset.Limits{MaxBytes: 1})
	if payload != nil || !errors.Is(err, rowset.ErrCapacity) || len(state.Calls()) != 0 {
		t.Fatalf("envelope budget JSON = %s, err = %v, calls = %d", payload, err, len(state.Calls()))
	}
}
