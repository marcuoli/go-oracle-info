package patches

import (
	"context"
	"errors"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"strings"
	"testing"
)

type coreFixtureReader struct {
	query func(context.Context, session.Statement) (session.Result, error)
	calls int
}

func (r *coreFixtureReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	r.calls++
	return r.query(ctx, s)
}
func coreReference(t *testing.T) reference.Reference {
	t.Helper()
	ref, err := reference.NewReference("unit", "core", "facts", 1)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func coreResult(columns string, values ...session.Value) session.Result {
	return session.Result{Columns: strings.Split(columns, ","), Rows: [][]session.Value{values}}
}
func coreText(value string) session.Value { return session.Value{Text: value} }

func TestHistoryMapsDocumentedFieldsAndOptionalBundleSeries(t *testing.T) {
	source := coreResult("ACTION_TIME,ACTION,NAMESPACE,VERSION,ID,COMMENTS", coreText("2026-11-15T01:02:03.123456"), coreText("UPGRADE"), coreText("SERVER"), coreText("11.2.0.4.0"), coreText("123"), coreText(" multiline\ncomment "))
	facts, err := DecodeHistory(source, HistoryOptions{OracleMajor: 11})
	if err != nil || len(facts.Rows) != 1 || facts.Rows[0].ActionTime.Text != "2026-11-15T01:02:03.123456" || facts.Rows[0].Namespace.Text != "SERVER" || facts.Rows[0].ID.Text != "123" || facts.Rows[0].Comments.Text != " multiline\ncomment " || facts.Rows[0].BundleSeries != nil {
		t.Fatalf("history: %+v %v", facts, err)
	}
	source.Columns = append(source.Columns, "BUNDLE_SERIES")
	source.Rows[0] = append(source.Rows[0], session.Value{Null: true})
	facts, err = DecodeHistory(source, HistoryOptions{OracleMajor: 19, IncludeBundleSeries: true})
	if err != nil || facts.Rows[0].BundleSeries == nil || !facts.Rows[0].BundleSeries.Null {
		t.Fatal("bundle NULL lost", err)
	}
	source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
	source.Rows[1][4] = coreText("1.5")
	facts, err = DecodeHistory(source, HistoryOptions{OracleMajor: 19, IncludeBundleSeries: true})
	if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("malformed late history published", err)
	}
}
func patchFixture(options SQLPatchOptions) session.Result {
	names := "PATCH_ID,PATCH_UID,ACTION,STATUS,ACTION_TIME,DESCRIPTION,LOGFILE,FLAGS"
	values := []session.Value{coreText("123"), coreText("456"), coreText("ROLLBACK"), coreText("WITH ERRORS"), coreText("2026-11-15T01:02:03.000001"), session.Value{Null: true}, coreText("private-log"), coreText("R")}
	if options.OracleMajor >= 18 || options.OracleMinor == 2 {
		names += ",INSTALL_ID"
		values = append(values, coreText("7"))
	}
	if options.OracleMajor == 12 {
		names += ",VERSION,BUNDLE_SERIES,BUNDLE_ID"
		values = append(values, coreText("12.1.0.2"), session.Value{Null: true}, coreText("123"))
	} else {
		names += ",PATCH_TYPE,SOURCE_VERSION,SOURCE_BUILD_DESCRIPTION,SOURCE_BUILD_TIMESTAMP,TARGET_VERSION,TARGET_BUILD_DESCRIPTION,TARGET_BUILD_TIMESTAMP,RU_LOGFILE"
		values = append(values, coreText("future"), coreText("19.1.0.0.0"), session.Value{Null: true}, session.Value{Null: true}, coreText("19.2.0.0.0"), session.Value{Null: true}, session.Value{Null: true}, session.Value{Null: true})
	}
	return coreResult(names, values...)
}
func TestSQLPatchVersionLayoutsAndNullableAbsentFields(t *testing.T) {
	for _, options := range []SQLPatchOptions{{OracleMajor: 12, OracleMinor: 1}, {OracleMajor: 12, OracleMinor: 2}, {OracleMajor: 18}, {OracleMajor: 19}, {OracleMajor: 26}} {
		source := patchFixture(options)
		facts, err := DecodeSQLPatch(source, options)
		if err != nil || len(facts.Rows) != 1 {
			t.Fatal(options, facts, err)
		}
		fact := facts.Rows[0]
		if fact.Action.Text != "ROLLBACK" || fact.Status.Text != "WITH ERRORS" || fact.ActionTime.Text != "2026-11-15T01:02:03.000001" || !fact.Description.Null {
			t.Fatalf("attempt fields lost: %+v", fact)
		}
		if options.OracleMajor == 12 {
			if fact.Version == nil || fact.Version.Text != "12.1.0.2" || fact.BundleSeries == nil || !fact.BundleSeries.Null || fact.PatchType != nil || fact.SourceVersion != nil {
				t.Fatalf("legacy absence: %+v", fact)
			}
		} else {
			if fact.Version != nil || fact.BundleID != nil || fact.PatchType == nil || fact.PatchType.Text != "future" || fact.SourceVersion == nil || fact.SourceVersion.Text != "19.1.0.0.0" || fact.RULogfile == nil || !fact.RULogfile.Null {
				t.Fatalf("modern fields: %+v", fact)
			}
		}
		if (fact.InstallID != nil) != (options.OracleMajor >= 18 || options.OracleMinor == 2) {
			t.Fatal("INSTALL_ID release mismatch")
		}
		source.Rows = append(source.Rows, append([]session.Value(nil), source.Rows[0]...))
		source.Rows[1][0] = coreText("9223372036854775808")
		facts, err = DecodeSQLPatch(source, options)
		if facts.Rows != nil || !errors.Is(err, ErrInvalidEvidence) {
			t.Fatal("late patch overflow published", err)
		}
	}
	for _, options := range []SQLPatchOptions{{OracleMajor: 11}, {OracleMajor: 12}, {OracleMajor: 12, OracleMinor: 3}, {OracleMajor: 19, OracleMinor: 1}} {
		if sql, err := SQLPatchStatement(options); sql != "" || !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("ambiguous source admitted: %v %s %v", options, sql, err)
		}
	}
}
func TestPatchReadersRetainFailureAndRejectPayload(t *testing.T) {
	ref := coreReference(t)
	failure := errors.New("private failure")
	reader := &coreFixtureReader{query: func(_ context.Context, s session.Statement) (session.Result, error) {
		if s.Reference != ref {
			t.Fatal("reference changed")
		}
		return session.Result{}, failure
	}}
	history, err := ReadHistory(t.Context(), reader, HistoryRequest{Options: HistoryOptions{OracleMajor: 19}, Reference: ref})
	if history.Rows != nil || !errors.Is(err, failure) || strings.Contains(err.Error(), "private") {
		t.Fatalf("history failed prefix: %+v %v", history, err)
	}
	patch, err := ReadSQLPatch(t.Context(), reader, SQLPatchRequest{Options: SQLPatchOptions{OracleMajor: 19}, Reference: ref})
	if patch.Rows != nil || !errors.Is(err, failure) || reader.calls != 2 {
		t.Fatalf("SQL patch failed prefix: %+v %v", patch, err)
	}
}
