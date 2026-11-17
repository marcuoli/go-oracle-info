package passwordfile_test

import (
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/passwordfile"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"testing"
)

func TestMetadataJSONRejectsTypedInvalidContainer(t *testing.T) {
	source := metadataResult(false)
	source.Rows[0][3] = session.Value{Text: "3"}
	if _, err := passwordfile.Decode(source, baseOptions); !errors.Is(err, passwordfile.ErrInvalidEvidence) {
		t.Fatal("typed setup", err)
	}
	step := metadataStep(false)
	step.Values[0][3] = int64(3)
	db, state := testsql.New(t, step)
	got, err := passwordfile.JSON(t.Context(), db, baseOptions, rowset.Limits{})
	if got != nil || !errors.Is(err, passwordfile.ErrInvalidEvidence) {
		t.Fatalf("wrong password-file scope published: %s %v", got, err)
	}
	if state.Closed() != 1 {
		t.Fatal("cursor not finalized", state.Closed())
	}
}
