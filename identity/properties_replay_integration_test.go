//go:build integration

package identity_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

// TestRecordedIdentityProperties replays private, complete native SQL captures.
// It opens no database and does not qualify a live Go reader/driver/cursor. The
// capture receipt separately identifies the actual SQL transport and scope.
func TestRecordedIdentityProperties(t *testing.T) {
	path := os.Getenv("ORACLE_INFO_IDENTITY_REPLAY")
	if path == "" {
		t.Skip("explicit private identity capture path required")
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1<<20 {
		t.Fatal("identity capture unavailable or over budget")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("identity capture read failed")
	}
	var captured struct {
		Records []struct {
			Name    string
			Queries map[string]session.Result
		}
	}
	if err := json.Unmarshal(data, &captured); err != nil || len(captured.Records) != 3 {
		t.Fatal("expected complete three-scope Enterprise capture")
	}
	expected := map[string]identity.Architecture{
		"enterprise-primary-root":    identity.CDB,
		"enterprise-primary-pdb":     identity.PDB,
		"enterprise-mounted-standby": identity.CDB,
	}
	for _, record := range captured.Records {
		t.Run(record.Name, func(t *testing.T) {
			want, exists := expected[record.Name]
			if !exists {
				t.Fatal("unexpected native scope")
			}
			delete(expected, record.Name)
			ctx := context.Background()
			versionSites, architectureSites, editionSites := versionRefs(t), architectureRefs(t), editionRefs(t)
			versionReader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{
				{ref: versionSites.Legacy, statement: legacyVersionSQL, result: record.Queries["legacy"]},
				{ref: versionSites.Full, statement: fullVersionSQL, result: record.Queries["full"]},
			}}
			version, err := identity.ReadVersion(ctx, versionReader, versionSites)
			versionReader.done()
			if err != nil || version.State != identity.Available || version.Precision != identity.FullPrecision || version.Fallback || version.Value != "23.26.1.0.0" {
				t.Fatalf("recorded numeric version/precision changed: %#v %v", version, err)
			}
			architectureReader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{
				{ref: architectureSites.Version, statement: legacyVersionSQL, result: record.Queries["legacy"]},
				{ref: architectureSites.Container, statement: identity.ConnectedArchitectureSQL, result: record.Queries["architecture"]},
			}}
			architecture, err := identity.ReadArchitecture(ctx, architectureReader, architectureSites)
			architectureReader.done()
			if err != nil || architecture.State != identity.Available || architecture.Value != want || architecture.Method != identity.DictionaryArchitecture || architecture.Fallback || architecture.DatabaseCDB.Value != "YES" {
				t.Fatalf("recorded parent/connected architecture changed: %#v %v", architecture, err)
			}
			editionReader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{
				{ref: editionSites.Version, statement: legacyVersionSQL, result: record.Queries["legacy"]},
				{ref: editionSites.Code, statement: editionCodeSQL, result: record.Queries["edition"]},
			}}
			edition, err := identity.ReadEdition(ctx, editionReader, editionSites)
			editionReader.done()
			if err != nil || edition.State != identity.Available || edition.Value != identity.EnterpriseEdition || edition.Code.Value != "EE" || edition.Precision != identity.ObservedEditionCode || edition.Fallback {
				t.Fatalf("recorded explicit edition changed: %#v %v", edition, err)
			}
		})
	}
	if len(expected) != 0 {
		t.Fatal("required native scope missing")
	}
}
