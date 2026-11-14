package identity_test

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestObservedReleaseSelection(t *testing.T) {
	t.Parallel()
	for _, major := range []int{0, 10, 11, 12, 17, 18, 19, 23, 26} {
		t.Run(strconv.Itoa(major), func(t *testing.T) {
			edition, err := identity.EditionStatement(major)
			if major < 12 {
				if edition != "" || !errors.Is(err, identity.ErrUnsupportedVersion) {
					t.Fatalf("edition %d: %q %v", major, edition, err)
				}
			} else if err != nil || edition != "SELECT EDITION AS EDITION FROM V$INSTANCE" {
				t.Fatalf("edition %d: %q %v", major, edition, err)
			}
			banners, err := identity.BannersStatement(major)
			if major < 11 {
				if banners != "" || !errors.Is(err, identity.ErrUnsupportedVersion) {
					t.Fatalf("banners %d: %q %v", major, banners, err)
				}
				return
			}
			want := "SELECT BANNER AS BANNER FROM V$VERSION ORDER BY BANNER"
			if major >= 18 {
				want = "SELECT BANNER AS BANNER, BANNER_FULL AS BANNER_FULL FROM V$VERSION ORDER BY BANNER"
			}
			if err != nil || banners != want {
				t.Fatalf("banners %d: %q %v", major, banners, err)
			}
			version, err := identity.VersionStatement(major)
			if err != nil || strings.Contains(version, "VERSION_FULL") != (major >= 18) {
				t.Fatalf("version %d: %q %v", major, version, err)
			}
		})
	}
}

func TestDecodeObservedScalars(t *testing.T) {
	t.Parallel()
	for _, decoder := range []struct {
		name, column string
		decode       func(session.Result) (identity.Text, error)
	}{
		{"edition", "EDITION", identity.DecodeEdition}, {"full version", "VERSION_FULL", identity.DecodeFullVersion},
	} {
		t.Run(decoder.name, func(t *testing.T) {
			for _, tt := range []struct {
				name string
				rows [][]session.Value
				want identity.Text
			}{
				{"exact", [][]session.Value{{{Text: "  unknown-SE2-or-23.26.1.0.0\n"}}}, identity.Text{State: identity.Available, Value: "  unknown-SE2-or-23.26.1.0.0\n"}},
				{"NULL", [][]session.Value{{{Null: true}}}, identity.Text{State: identity.Null}},
				{"empty text", [][]session.Value{{{}}}, identity.Text{State: identity.Available}},
				{"no rows", nil, identity.Text{State: identity.NoRows}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := decoder.decode(session.Result{Columns: []string{decoder.column}, Rows: tt.rows})
					if err != nil || got != tt.want {
						t.Fatalf("got %+v %v, want %+v", got, err, tt.want)
					}
				})
			}
			if _, err := decoder.decode(session.Result{Columns: []string{"VERSION"}}); !errors.Is(err, identity.ErrInvalidEvidence) {
				t.Fatalf("wrong projection: %v", err)
			}
		})
	}
}

func TestDecodeBannersPreservesRowsAndAbsentFull(t *testing.T) {
	t.Parallel()
	source := "Oracle Database Enterprise Edition\nVersion 19.30.0.0.0\n "
	r := session.Result{Columns: []string{"banner_full", "banner"}, Rows: [][]session.Value{{{Text: source}, {Text: "legacy"}}, {{Null: true}, {Null: true}}, {{Text: source}, {Text: "legacy"}}}}
	full := identity.Text{State: identity.Available, Value: source}
	null := identity.Text{State: identity.Null}
	want := []identity.VersionBanner{{Banner: identity.Text{State: identity.Available, Value: "legacy"}, Full: &full}, {Banner: null, Full: &null}, {Banner: identity.Text{State: identity.Available, Value: "legacy"}, Full: &full}}
	got, err := identity.DecodeBanners(r, 19)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v %v, want %+v", got, err, want)
	}
	got[0].Full.Value = "changed"
	if got[2].Full.Value != source || r.Rows[0][0].Text != source {
		t.Fatal("banner rows share mutable fact storage")
	}
	legacy, err := identity.DecodeBanners(session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{Text: source}}}}, 11)
	if err != nil || len(legacy) != 1 || legacy[0].Full != nil || legacy[0].Banner.Value != source {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
	empty, err := identity.DecodeBanners(session.Result{Columns: []string{"BANNER"}}, 11)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
}

func TestDecodeBannersRejectsIncompleteShapes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		result session.Result
		major  int
		want   error
	}{
		{"unknown release", session.Result{}, 0, identity.ErrUnsupportedVersion},
		{"missing full", session.Result{Columns: []string{"BANNER"}}, 19, identity.ErrInvalidEvidence},
		{"unexpected full", session.Result{Columns: []string{"BANNER", "BANNER_FULL"}}, 11, identity.ErrInvalidEvidence},
		{"duplicate", session.Result{Columns: []string{"BANNER", "banner"}}, 19, identity.ErrInvalidEvidence},
		{"wrong", session.Result{Columns: []string{"BANNER", "OTHER"}}, 19, identity.ErrInvalidEvidence},
		{"late short row", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{Text: "valid"}}, nil}}, 11, identity.ErrInvalidEvidence},
		{"long row", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{}, {}}}}, 11, identity.ErrInvalidEvidence},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := identity.DecodeBanners(tt.result, tt.major)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("got %+v %v, want nil %v", got, err, tt.want)
			}
		})
	}
}
