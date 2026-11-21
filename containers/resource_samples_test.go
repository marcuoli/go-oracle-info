package containers

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
)

func TestResourceViewsAreGatedIndependently(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"9.2.0.8.0", false}, {"10.1.0.5.0", false}, {"10.2.0.5.0", false},
		{"11.1.0.7.0", false}, {"11.2.0.4.0", false},
		{"12.1.0.1.0", false}, {"12.1.0.2.0", false}, {"12.2.0.1.0", true},
		{"18.20.0.0.0", true}, {"19.32.0.0.0", true}, {"21.21.0.0.0", true},
		{"23.4.0.0.0", true}, {"23.9.0.0.0", true}, {"23.26.1.0.0", true}, {"23.26.4.0.0", true},
	}
	for _, tc := range cases {
		options := ResourceOptions{tc.version, Root}
		metric, metricErr := ResourceSamplesStatement(options)
		usage, usageErr := ResourceUsageStatement(options)
		if !tc.want {
			if metric != "" || usage != "" || !errors.Is(metricErr, ErrUnsupportedVersion) || !errors.Is(usageErr, ErrUnsupportedVersion) {
				t.Fatal(tc.version, metricErr, usageErr)
			}
			continue
		}
		if metricErr != nil || usageErr != nil ||
			!strings.Contains(metric, "FROM GV$RSRCPDBMETRIC v") || !strings.Contains(usage, "FROM GV$RSRC_PDB v") ||
			!strings.Contains(metric, "EXTRACT(YEAR FROM CAST(v.BEGIN_TIME AS TIMESTAMP))") ||
			!strings.Contains(metric, "EXTRACT(SECOND FROM CAST(v.END_TIME AS TIMESTAMP))") ||
			!strings.Contains(metric, " AS INTSIZE_CSEC") || !strings.Contains(metric, " AS AVG_QUEUED_PARALLEL_STMTS") ||
			!strings.Contains(usage, " AS SGA_BYTES") || !strings.Contains(usage, " AS PGA_BYTES") {
			t.Fatalf("%s samples=%q usage=%q errors=%v %v", tc.version, metric, usage, metricErr, usageErr)
		}
	}
}

func TestResourceSampleJSONPreservesWindowAndExactRates(t *testing.T) {
	options := ResourceOptions{"23.26.1.0.0", CurrentContainer}
	p, err := resourceSamplesPlan(options)
	if err != nil {
		t.Fatal(err)
	}
	cols := make([]string, len(p.schema))
	for i, c := range p.schema {
		cols[i] = c.Name
	}
	value := []driver.Value{int64(2), int64(3), int64(3), "2026-11-21T10:10:00", "2026-11-21T10:11:00", "6000",
		"0.1250000000000000001", ".3", nil, "600.25", "10.875", "CASEPlan", ".02E-4", "0"}
	step := testsql.Step{SQL: p.sql, Columns: cols, Values: [][]driver.Value{value}}
	db, state := testsql.New(t, step)
	payload, err := ResourceSamplesJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), `[2,3,3,"2026-11-21T10:10:00","2026-11-21T10:11:00","6000","0.1250000000000000001",".3",null,"600.25","10.875","CASEPlan",".02E-4","0"]`) ||
		state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
	wrongWindow := append([]driver.Value(nil), value...)
	wrongWindow[4] = "2026-11-31T10:11:00"
	step.Values = append(step.Values, wrongWindow)
	db, state = testsql.New(t, step)
	payload, err = ResourceSamplesJSON(t.Context(), db, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatal(string(payload), err, state.Closed())
	}
}

func TestResourceUsagePreservesExactNullableGauges(t *testing.T) {
	options := ResourceOptions{"12.2.0.1.0", Root}
	p, err := resourceUsagePlan(options)
	if err != nil {
		t.Fatal(err)
	}
	src := scopeFixture(p, sessionValue("2"), sessionValue("3"), sessionValue("1"),
		sessionValue("18446744073709551616"), sessionValue(""))
	src.Rows[0][4].Null = true
	result, err := DecodeResourceUsage(src, options)
	if err != nil || len(result.Rows) != 1 || result.Provenance.Complete || result.Rows[0].SGABytes.Text != "18446744073709551616" ||
		!result.Rows[0].PGABytes.Null || result.Rows[0].InstanceID.Text != "2" {
		t.Fatal(result, err)
	}
}
