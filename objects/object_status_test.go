package objects

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestObjectStatusStatementUsesBoundExactKeys(t *testing.T) {
	options := ObjectStatusOptions{OracleRelease: "23.26.1.0.0", Keys: []ObjectStatusKey{
		{Owner: "Quo'ted", ObjectType: "TABLE", ObjectName: "Mixed Name"},
		{Owner: "Other", ObjectType: "INDEX", ObjectName: "Idx", SubobjectName: new(session.Value{Null: true})},
		{Owner: "Other", ObjectType: "TABLE PARTITION", ObjectName: "Part Table", SubobjectName: new(coreText("Quoted Part"))},
	}}
	stmt, args, err := ObjectStatusStatement(options)
	want := []any{"Quo'ted", "TABLE", "Mixed Name", "Other", "INDEX", "Idx", "Other", "TABLE PARTITION", "Part Table", "Quoted Part"}
	if err != nil || !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected bound request: %v, %v", args, err)
	}
	for _, fragment := range []string{
		"WITH requested_objects AS (SELECT CAST(:1 AS VARCHAR2(128 CHAR)) AS OWNER_NAME", "WHERE EXISTS (", "FROM DBA_OBJECTS o", "o.OWNER = k.OWNER_NAME",
		"NLSSORT(o.OBJECT_NAME, 'NLS_SORT=BINARY') = NLSSORT(k.OBJECT_NAME, 'NLS_SORT=BINARY')",
		"0 AS SUBOBJECT_MODE, CAST(NULL AS VARCHAR2(128 CHAR))", "1 AS SUBOBJECT_MODE, CAST(NULL AS VARCHAR2(128 CHAR))",
		"2 AS SUBOBJECT_MODE, CAST(:10 AS VARCHAR2(128 CHAR))", "o.SUBOBJECT_NAME IS NULL", "o.EDITION_NAME AS EDITION_NAME",
	} {
		if !strings.Contains(stmt, fragment) {
			t.Errorf("missing %q from bounded SQL", fragment)
		}
	}
	if strings.Contains(stmt, "Quo'ted") || strings.Contains(stmt, "Quoted Part") ||
		strings.Contains(stmt, "'INVALID'") || strings.Contains(stmt, "'VALID'") {
		t.Fatal("statement interpolated a key or constrained raw status")
	}
}

func TestObjectStatusReleaseAndBatchAdmission(t *testing.T) {
	one := []ObjectStatusKey{{Owner: "APP", ObjectType: "TABLE", ObjectName: "T"}}
	for _, version := range []string{"9.2", "10.1", "10.2", "26.0.0.0.0"} {
		stmt, args, err := ObjectStatusStatement(ObjectStatusOptions{OracleRelease: version, Keys: one})
		if !errors.Is(err, ErrUnsupportedVersion) || stmt != "" || args != nil {
			t.Fatalf("%s selected unsupported SQL", version)
		}
	}
	for _, version := range []string{"11.1", "11.2.0.4.0", "12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.0", "19.32", "21.0", "23.9", "23.26.1.0.0"} {
		stmt, _, err := ObjectStatusStatement(ObjectStatusOptions{OracleRelease: version, Keys: one})
		if err != nil || !strings.HasPrefix(stmt, "WITH requested_objects AS (SELECT ") || strings.Contains(stmt, "o.EDITION_NAME AS EDITION_NAME") != (version != "11.1") {
			t.Fatalf("%s selected wrong edition projection: %v", version, err)
		}
	}
	maxKeys := make([]ObjectStatusKey, MaxObjectStatusKeys)
	for i := range maxKeys {
		maxKeys[i] = one[0]
	}
	_, args, err := ObjectStatusStatement(ObjectStatusOptions{OracleRelease: "19.32", Keys: maxKeys})
	if err != nil || len(args) != 3*MaxObjectStatusKeys {
		t.Fatalf("maximum batch rejected or wrong binds: %d, %v", len(args), err)
	}
	for _, keys := range [][]ObjectStatusKey{
		nil,
		append(maxKeys, one[0]),
		{{Owner: "APP", ObjectType: "TABLE", ObjectName: "T", SubobjectName: new(coreText(""))}},
		{{Owner: "APP", ObjectType: "TABLE", ObjectName: "T", SubobjectName: new(session.Value{Null: true, Text: "bad"})}},
		{{Owner: "", ObjectType: "TABLE", ObjectName: "T"}},
	} {
		if stmt, _, err := ObjectStatusStatement(ObjectStatusOptions{OracleRelease: "19.32", Keys: keys}); stmt != "" || !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("invalid batch admitted")
		}
	}
	if _, _, err := ObjectStatusStatement(ObjectStatusOptions{OracleRelease: "26ai", Keys: one}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("branding admitted as numeric release: %v", err)
	}
}

func TestObjectStatusDecodeRetainsAmbiguityAndRawValues(t *testing.T) {
	options := ObjectStatusOptions{OracleRelease: "19.32", Keys: []ObjectStatusKey{
		{Owner: "App", ObjectType: "TABLE", ObjectName: "Mixed"},
		{Owner: "App", ObjectType: "VIEW", ObjectName: "Missing"},
	}}
	null := session.Value{Null: true}
	input := coreResult("OWNER,OBJECT_TYPE,OBJECT_NAME,SUBOBJECT_NAME,STATUS,EDITION_NAME",
		coreText("App"), coreText("TABLE"), coreText("Mixed"), null, coreText("VALID"), null)
	input.Rows = append(input.Rows, input.Rows[0],
		[]session.Value{coreText("App"), coreText("TABLE"), coreText("Mixed"), coreText("Part"), coreText("OTHER"), coreText("Edition")},
		[]session.Value{coreText("App"), coreText("TABLE"), coreText("Mixed"), null, null, null})
	got, err := DecodeObjectStatus(input, options)
	if err != nil || got.Provenance.Complete || len(got.Provenance.Keys) != 2 || len(got.Rows) != 4 ||
		got.Rows[0].Status.Text != "VALID" || !reflect.DeepEqual(got.Rows[0], got.Rows[1]) ||
		!got.Rows[0].SubobjectName.Null || got.Rows[0].EditionName == nil || !got.Rows[0].EditionName.Null ||
		got.Rows[2].SubobjectName.Text != "Part" || got.Rows[2].Status.Text != "OTHER" ||
		got.Rows[2].EditionName == nil || got.Rows[2].EditionName.Text != "Edition" || !got.Rows[3].Status.Null {
		t.Fatalf("lost a candidate, status or provenance: %+v, %v", got, err)
	}
	for _, mismatch := range []ObjectStatusKey{
		{Owner: "APP", ObjectType: "TABLE", ObjectName: "Mixed"},
		{Owner: "App", ObjectType: "VIEW", ObjectName: "Mixed"},
		{Owner: "App", ObjectType: "TABLE", ObjectName: "Mixed", SubobjectName: new(null)},
		{Owner: "App", ObjectType: "TABLE", ObjectName: "Mixed", SubobjectName: new(coreText("PART"))},
	} {
		wrong := ObjectStatusOptions{OracleRelease: "19.32", Keys: []ObjectStatusKey{mismatch}}
		if result, err := DecodeObjectStatus(input, wrong); !errors.Is(err, ErrInvalidEvidence) || result.Rows != nil {
			t.Fatalf("disallowed or ambiguous selection leaked a prefix: %+v, %v", result, err)
		}
	}
	options.OracleRelease = "11.1"
	input.Columns, input.Rows = input.Columns[:5], [][]session.Value{input.Rows[0][:5]}
	older, err := DecodeObjectStatus(input, options)
	if err != nil || len(older.Rows) != 1 || older.Rows[0].EditionName != nil {
		t.Fatalf("legacy absence of edition metadata lost: %+v, %v", older, err)
	}
	input.Rows = nil
	empty, err := DecodeObjectStatus(input, options)
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("successful absence not retained: %+v, %v", empty, err)
	}
}

func TestReadObjectStatusRejectsFailedOrOversizeEvidence(t *testing.T) {
	options := ObjectStatusOptions{OracleRelease: "19.32", Keys: []ObjectStatusKey{{Owner: "App", ObjectType: "TABLE", ObjectName: "T"}}}
	ref := coreReference(t)
	input := coreResult("OWNER,OBJECT_TYPE,OBJECT_NAME,SUBOBJECT_NAME,STATUS,EDITION_NAME",
		coreText("App"), coreText("TABLE"), coreText("T"), session.Value{Null: true}, coreText("VALID"), session.Value{Null: true})
	stmt, args, err := ObjectStatusStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	reader := &coreFixtureReader{query: func(_ context.Context, s session.Statement) (session.Result, error) {
		if s.Reference != ref || s.Text != stmt || !reflect.DeepEqual(s.Args, args) {
			t.Fatalf("unexpected admitted statement: %+v", s)
		}
		return input, nil
	}}
	request := ObjectStatusRequest{Options: options, Reference: ref}
	got, err := ReadObjectStatus(t.Context(), reader, request)
	if err != nil || !got.Provenance.Complete || reader.calls != 1 || len(got.Rows) != 1 || got.Rows[0].Status.Text != "VALID" {
		t.Fatalf("complete read did not preserve status: %+v, %v", got, err)
	}
	request.Limits.MaxRows = 1
	input.Rows = append(input.Rows, input.Rows[0])
	if got, err := ReadObjectStatus(t.Context(), reader, request); !errors.Is(err, rowset.ErrCapacity) || got.Rows != nil {
		t.Fatalf("over-budget read yielded status: %+v, %v", got, err)
	}
	request.Limits = rowset.Limits{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader.query = func(context.Context, session.Statement) (session.Result, error) { cancel(); return input, nil }
	if got, err := ReadObjectStatus(ctx, reader, request); !errors.Is(err, context.Canceled) || got.Rows != nil {
		t.Fatalf("canceled read yielded status: %+v, %v", got, err)
	}
	failure := errors.New("reader failed after a prefix")
	reader.query = func(context.Context, session.Statement) (session.Result, error) { return input, failure }
	if got, err := ReadObjectStatus(t.Context(), reader, request); !errors.Is(err, failure) || got.Rows != nil {
		t.Fatalf("failed read yielded status: %+v, %v", got, err)
	}
}

func TestObjectStatusNativeAndBoundedJSON(t *testing.T) {
	options := ObjectStatusOptions{OracleRelease: "23.26.1", Keys: []ObjectStatusKey{{Owner: "App", ObjectType: "TABLE", ObjectName: "T"}}}
	stmt, _, err := ObjectStatusStatement(options)
	if err != nil {
		t.Fatal(err)
	}
	step := testsql.Step{
		SQL: stmt, Columns: []string{"OWNER", "OBJECT_TYPE", "OBJECT_NAME", "SUBOBJECT_NAME", "STATUS", "EDITION_NAME"},
		Values: [][]driver.Value{{"App", "TABLE", "T", nil, "VALID", nil}},
	}
	db, state := testsql.New(t, step)
	rows, err := ObjectStatusRows(t.Context(), db, options)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var owner, typ, name, status string
	var subobject, edition *string
	if !rows.Next() || rows.Scan(&owner, &typ, &name, &subobject, &status, &edition) != nil ||
		owner != "App" || typ != "TABLE" || name != "T" || subobject != nil || status != "VALID" || edition != nil ||
		rows.Next() || rows.Err() != nil || rows.Close() != nil || state.Closed() != 1 {
		t.Fatal("native cursor changed dictionary candidate or ownership")
	}
	db, state = testsql.New(t, step)
	payload, err := ObjectStatusJSON(t.Context(), db, options, rowset.Limits{})
	var envelope struct {
		Provenance ObjectStatusProvenance
		Data       struct{ Rows [][]*string }
	}
	if err != nil || json.Unmarshal(payload, &envelope) != nil || !envelope.Provenance.Complete || state.Closed() != 1 ||
		len(envelope.Data.Rows) != 1 || len(envelope.Data.Rows[0]) != 6 ||
		envelope.Data.Rows[0][3] != nil || envelope.Data.Rows[0][4] == nil || *envelope.Data.Rows[0][4] != "VALID" {
		t.Fatalf("incorrect completed JSON: %s, %v", payload, err)
	}
	step.NextErr = errors.New("late cursor failure")
	db, state = testsql.New(t, step)
	if payload, err := ObjectStatusJSON(t.Context(), db, options, rowset.Limits{}); !errors.Is(err, step.NextErr) || payload != nil || state.Closed() != 1 {
		t.Fatalf("failed native cursor yielded JSON: %s, %v", payload, err)
	}
}
