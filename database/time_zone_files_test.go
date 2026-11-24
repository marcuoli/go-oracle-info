package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-session/session"
)

func TestTimeZoneFilesVersionBoundary(t *testing.T) {
	for _, tc := range []struct {
		version         string
		supported, full bool
	}{
		{"9.2.0.8.0", false, false}, {"10.1.0.5.0", true, false}, {"10.2.0.5.0", true, false},
		{"11.1.0.7.0", true, false}, {"11.2.0.4.0", true, false},
		{"12.1.0.1.0", true, false}, {"12.1.0.2.0", true, false}, {"12.2.0.1.0", true, true},
		{"18.1.0.0.0", true, true}, {"19.32.0.0.0", true, true}, {"21.21.0.0.0", true, true},
		{"23.9.0.0.0", true, true}, {"23.26.4.0.0", true, true}, {"26ai", false, false},
	} {
		sql, err := TimeZoneFilesStatement(TimeZoneFilesOptions{tc.version})
		if !tc.supported {
			if !errors.Is(err, ErrUnsupportedVersion) || sql != "" {
				t.Fatal(tc, sql, err)
			}
			continue
		}
		if err != nil || !strings.Contains(sql, "TO_CHAR(v.VERSION") || !strings.Contains(sql, "FROM V$TIMEZONE_FILE") ||
			strings.Contains(sql, "v.FILENAME AS FILENAME") != tc.full || strings.Contains(sql, "AS SOURCE_CON_ID") != (tc.full || strings.HasPrefix(tc.version, "12.1.")) {
			t.Fatal(tc, sql, err)
		}
	}
}

func TestTimeZoneFilesAbsentColumnsAndConnectedProvenance(t *testing.T) {
	legacy := coreResult("VERSION", session.Value{Text: "42"})
	facts, err := DecodeTimeZoneFiles(legacy, TimeZoneFilesOptions{"11.2.0.4.0"})
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].Version.Text != "42" || facts.Rows[0].Filename != nil ||
		facts.Rows[0].ContainerID != nil || facts.Rows[0].SourceContainerID != nil {
		t.Fatal(facts, err)
	}
	current := coreResult("VERSION,FILENAME,CON_ID,SOURCE_CON_ID", session.Value{Text: "43"}, session.Value{Null: true},
		session.Value{Text: "0"}, session.Value{Text: "1"})
	facts, err = ReadTimeZoneFiles(t.Context(), datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
		return current, nil
	}), TimeZoneFilesRequest{
		Options: TimeZoneFilesOptions{"23.26.1.0.0"}, Reference: coreReference(t),
	})
	if err != nil || len(facts.Rows) != 1 || !facts.Provenance.Complete || !facts.Rows[0].Filename.Null ||
		facts.Rows[0].ContainerID.Text != "0" || facts.Rows[0].SourceContainerID.Text != "1" {
		t.Fatal(facts, err)
	}
}
