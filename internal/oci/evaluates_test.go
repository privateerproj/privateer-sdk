package oci

import (
	"strings"
	"testing"

	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

func TestValidateForPublish_RejectsEmptyEvaluates(t *testing.T) {
	// This is the exact gap that caused the live 422: an index with no evaluates.
	p := AssembleParams{
		Coordinate: "sandbox/hello",
		Version:    "0.1.0",
		License:    "Apache-2.0",
		Binaries:   []PlatformBinary{{OS: "linux", Arch: "amd64", Path: "/x", Entrypoint: "hello"}},
	}
	err := ValidateForPublish(p)
	if err == nil {
		t.Fatal("expected preflight to reject an empty evaluates list (the live 422)")
	}
	if !strings.Contains(err.Error(), "evaluates") {
		t.Errorf("error should name evaluates: %v", err)
	}
}

func TestValidateForPublish_AcceptsWellFormed(t *testing.T) {
	p := AssembleParams{
		Coordinate: "sandbox/hello",
		Version:    "0.1.0",
		License:    "Apache-2.0",
		Binaries:   []PlatformBinary{{OS: "linux", Arch: "amd64", Path: "/x", Entrypoint: "hello"}},
		Evaluates:  []pluginspec.Evaluate{{Catalog: "sandbox/example", CatalogVersion: "2026.01", RequirementIDs: []string{"SBX.1"}}},
	}
	if err := ValidateForPublish(p); err != nil {
		t.Fatalf("well-formed plugin should pass preflight: %v", err)
	}
}

func TestValidateForPublish_RejectsMissingLicense(t *testing.T) {
	// grc.store requires a license; an otherwise well-formed plugin without one
	// must be rejected at preflight.
	p := AssembleParams{
		Coordinate: "sandbox/hello",
		Version:    "0.1.0",
		Binaries:   []PlatformBinary{{OS: "linux", Arch: "amd64", Path: "/x", Entrypoint: "hello"}},
		Evaluates:  []pluginspec.Evaluate{{Catalog: "sandbox/example", CatalogVersion: "2026.01", RequirementIDs: []string{"SBX.1"}}},
	}
	err := ValidateForPublish(p)
	if err == nil {
		t.Fatal("expected preflight to reject a plugin with no license")
	}
	if !strings.Contains(err.Error(), "license") {
		t.Errorf("error should name the license: %v", err)
	}
}

func TestValidateForPublish_RejectsBadFields(t *testing.T) {
	base := func() AssembleParams {
		return AssembleParams{
			Coordinate: "sandbox/hello",
			Version:    "0.1.0",
			License:    "Apache-2.0",
			Binaries:   []PlatformBinary{{OS: "linux", Arch: "amd64", Path: "/x", Entrypoint: "hello"}},
			Evaluates:  []pluginspec.Evaluate{{Catalog: "sandbox/example", CatalogVersion: "2026.01", RequirementIDs: []string{"SBX.1"}}},
		}
	}
	t.Run("non-namespaced catalog", func(t *testing.T) {
		p := base()
		p.Evaluates[0].Catalog = "example" // no namespace
		if err := ValidateForPublish(p); err == nil {
			t.Error("expected rejection of a non-namespaced catalog")
		}
	})
	t.Run("no requirement ids", func(t *testing.T) {
		p := base()
		p.Evaluates[0].RequirementIDs = nil
		if err := ValidateForPublish(p); err == nil {
			t.Error("expected rejection of empty requirement_ids")
		}
	})
	t.Run("missing entrypoint", func(t *testing.T) {
		p := base()
		p.Binaries[0].Entrypoint = ""
		if err := ValidateForPublish(p); err == nil {
			t.Error("expected rejection of a missing entrypoint")
		}
	})
	t.Run("no binaries", func(t *testing.T) {
		p := base()
		p.Binaries = nil
		if err := ValidateForPublish(p); err == nil {
			t.Error("expected rejection of no binaries")
		}
	})
}
