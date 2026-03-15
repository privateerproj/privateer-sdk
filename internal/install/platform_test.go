package install

import (
	"runtime"
	"testing"
)

func TestInferGitHubReleaseBase(t *testing.T) {
	tests := []struct {
		source, latest string
		wantBase       string
		wantOK         bool
	}{
		{"https://github.com/owner/repo", "0.19.2", "https://github.com/owner/repo/releases/download/v0.19.2", true},
		{"https://github.com/owner/repo", "v0.19.2", "https://github.com/owner/repo/releases/download/v0.19.2", true},
		{"https://github.com/owner/repo/", "1.0", "https://github.com/owner/repo/releases/download/v1.0", true},
		{"https://github.com/owner/repo.git", "2.0", "https://github.com/owner/repo/releases/download/v2.0", true},
		{"https://github.com/owner/repo", "", "", false},
		{"", "0.19.2", "", false},
		{"https://gitlab.com/owner/repo", "0.19.2", "", false},
		{"https://example.com/not-github", "0.19.2", "", false},
	}
	for _, tt := range tests {
		base, ok := InferGitHubReleaseBase(tt.source, tt.latest)
		if ok != tt.wantOK || base != tt.wantBase {
			t.Errorf("InferGitHubReleaseBase(%q, %q) = %q, %v; want %q, %v", tt.source, tt.latest, base, ok, tt.wantBase, tt.wantOK)
		}
	}
}

func TestInferArtifactFilename(t *testing.T) {
	// Valid binary name on current platform should succeed
	got, err := InferArtifactFilename("pvtr-github-repo-scanner")
	if err != nil {
		t.Fatalf("InferArtifactFilename: %v", err)
	}
	// Expect {name}_{OS}_{arch}.ext
	if got == "" {
		t.Fatal("expected non-empty filename")
	}
	// Check format based on current runtime
	var wantExt string
	if runtime.GOOS == "windows" {
		wantExt = ".zip"
	} else {
		wantExt = ".tar.gz"
	}
	if len(got) < len(wantExt) || got[len(got)-len(wantExt):] != wantExt {
		t.Errorf("filename %q should end with %q", got, wantExt)
	}
	if runtime.GOOS == "darwin" && (len(got) < 10 || got[len(got)-10:len(got)-7] != "_all") {
		// Darwin uses _all for arch
		t.Logf("darwin artifact: %s", got)
	}
}

func TestInferArtifactFilename_EmptyName(t *testing.T) {
	_, err := InferArtifactFilename("")
	if err == nil {
		t.Fatal("expected error for empty binary name")
	}
	_, err = InferArtifactFilename("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}
}

func TestInferArtifactFilename_TrimSpace(t *testing.T) {
	got, err := InferArtifactFilename("  pvtr-plugin  ")
	if err != nil {
		t.Fatalf("InferArtifactFilename: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty filename")
	}
	// Should start with binary name (trimmed)
	if len(got) < 11 || got[:11] != "pvtr-plugin" {
		t.Errorf("filename should start with pvtr-plugin: %q", got)
	}
}
