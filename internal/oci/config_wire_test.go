package oci

import (
	"encoding/json"
	"testing"

	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

// TestPluginConfigWireFormat freezes the exact JSON bytes of the signed config
// blob. Adopting pluginspec.Config (ADR-0035) dropped the ,omitempty our old
// oci.PluginConfig carried on Evaluates — this must NOT change the published wire
// format. Real plugins always carry non-empty evaluates (ValidateForPublish
// rejects empty; see evaluates_test.go), so the omitempty difference can never
// reach a published, content-addressed config blob. This pins field order, JSON
// tags, and the non-empty-evaluates shape against drift in either pluginspec or
// our assembly.
func TestPluginConfigWireFormat(t *testing.T) {
	cfg := pluginspec.Config{
		Plugin:     "sandbox/sandbox-plugin",
		Version:    "0.26.1-rc",
		License:    "Apache-2.0",
		Platform:   pluginspec.Platform{OS: "linux", Arch: "amd64"},
		Entrypoint: "github-repo",
		Protocol:   defaultProtocol,
		Evaluates: []pluginspec.Evaluate{
			{Catalog: "openssf/osps-baseline", CatalogVersion: "2025.10", RequirementIDs: []string{"OSPS-AC-01", "OSPS-AC-02"}},
		},
	}
	got, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"plugin":"sandbox/sandbox-plugin","version":"0.26.1-rc","license":"Apache-2.0","platform":{"os":"linux","arch":"amd64"},"entrypoint":"github-repo","protocol":"netrpc","evaluates":[{"catalog":"openssf/osps-baseline","catalog_version":"2025.10","requirement_ids":["OSPS-AC-01","OSPS-AC-02"]}]}`
	if string(got) != want {
		t.Fatalf("config-blob wire format drift:\n got  %s\n want %s", got, want)
	}
}

// TestPluginConfigWireFormat_EmptyEvaluatesShape documents the post-adoption
// marshal behavior for nil/empty evaluates. pluginspec.Config dropped the
// `,omitempty` our old oci.PluginConfig had, so an empty list now serializes as
// "evaluates":null / "evaluates":[] rather than being omitted. This shape CANNOT
// reach a published, content-addressed config blob — ValidateForPublish rejects
// empty evaluates before assembly, and only the testing-only --registry path
// skips that gate. Pinned here so any future omitempty change is a visible test
// change rather than a silent digest-affecting drift.
func TestPluginConfigWireFormat_EmptyEvaluatesShape(t *testing.T) {
	base := pluginspec.Config{
		Plugin:     "ns/p",
		Version:    "1.0.0",
		Platform:   pluginspec.Platform{OS: "linux", Arch: "amd64"},
		Entrypoint: "p",
		Protocol:   defaultProtocol,
	}
	nilCfg := base // Evaluates left nil
	emptyCfg := base
	emptyCfg.Evaluates = []pluginspec.Evaluate{}

	gotNil, err := json.Marshal(nilCfg)
	if err != nil {
		t.Fatal(err)
	}
	const wantNil = `{"plugin":"ns/p","version":"1.0.0","license":"","platform":{"os":"linux","arch":"amd64"},"entrypoint":"p","protocol":"netrpc","evaluates":null}`
	if string(gotNil) != wantNil {
		t.Errorf("nil Evaluates shape drift:\n got  %s\n want %s", gotNil, wantNil)
	}

	gotEmpty, err := json.Marshal(emptyCfg)
	if err != nil {
		t.Fatal(err)
	}
	const wantEmpty = `{"plugin":"ns/p","version":"1.0.0","license":"","platform":{"os":"linux","arch":"amd64"},"entrypoint":"p","protocol":"netrpc","evaluates":[]}`
	if string(gotEmpty) != wantEmpty {
		t.Errorf("empty Evaluates shape drift:\n got  %s\n want %s", gotEmpty, wantEmpty)
	}
}
