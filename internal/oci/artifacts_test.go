package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// loadFixtureArtifacts decodes the committed real GoReleaser artifacts.json
// (from `goreleaser build` of pvtr-github-repo-scanner). Using a real fixture,
// not a hand-written one, keeps the parser honest against GoReleaser v2's
// actual schema (esp. the darwin universal entry).
func loadFixtureArtifacts(t *testing.T) []goreleaserArtifact {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "goreleaser-artifacts.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var arts []goreleaserArtifact
	if err := json.Unmarshal(data, &arts); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return arts
}

// makeOriginalDistTree creates stub binary files in a temporary directory that
// mirrors the original GoReleaser layout: <tmpdir>/dist/<id>_<target>/<bin>.
// The returned distDir is <tmpdir>/dist. Callers use this to test
// resolvePlatformBinaries with the original (candidate 1) resolution path.
func makeOriginalDistTree(t *testing.T, arts []goreleaserArtifact) (distDir string) {
	t.Helper()
	root := t.TempDir()
	distDir = filepath.Join(root, "dist")
	for _, a := range arts {
		if a.Type != artifactTypeBinary {
			continue
		}
		// a.Path is like "dist/pvtr-github-repo-scanner_linux_amd64_v1/github-repo"
		dst := filepath.Join(root, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, []byte("stub"), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", dst, err)
		}
	}
	return distDir
}

// makeMovedDistTree creates stub binary files in a temporary directory that
// mirrors a moved/downloaded dist layout: <tmpdir>/<distName>/<id>_<target>/<bin>,
// i.e. the original "dist/" first-segment is stripped. The returned distDir is
// <tmpdir>/<distName>. Callers use this to test the moved-dist (candidate 2)
// resolution path.
func makeMovedDistTree(t *testing.T, arts []goreleaserArtifact, distName string) (distDir string) {
	t.Helper()
	root := t.TempDir()
	distDir = filepath.Join(root, distName)
	for _, a := range arts {
		if a.Type != artifactTypeBinary {
			continue
		}
		// Strip the leading "dist/" segment from the recorded path, then place
		// the stub under <distDir>/<rest>, simulating a downloaded dist tree.
		stripped := filepath.FromSlash(stripFirstSegment(a.Path))
		dst := filepath.Join(distDir, stripped)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, []byte("stub"), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", dst, err)
		}
	}
	return distDir
}

func TestResolvePlatformBinaries_RealFixture(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	distDir := makeOriginalDistTree(t, arts)
	bins, err := resolvePlatformBinaries(arts, distDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// The fixture builds: linux/{386,amd64,arm64}, windows/{386,amd64,arch64},
	// and darwin/all (universal). darwin/all must re-expand to amd64+arm64, so
	// the count is 6 non-darwin + 2 darwin = 8.
	got := map[string]string{} // "os/arch" -> path
	for _, b := range bins {
		got[b.OS+"/"+b.Arch] = b.Path
	}
	want := []string{
		"linux/386", "linux/amd64", "linux/arm64",
		"windows/386", "windows/amd64", "windows/arm64",
		"darwin/amd64", "darwin/arm64",
	}
	if len(bins) != len(want) {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("expected %d platforms, got %d: %v", len(want), len(bins), keys)
	}
	for _, p := range want {
		if _, ok := got[p]; !ok {
			t.Errorf("missing platform %s", p)
		}
	}
}

func TestResolvePlatformBinaries_DarwinUniversalSharesOnePath(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	distDir := makeOriginalDistTree(t, arts)
	bins, err := resolvePlatformBinaries(arts, distDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var darwinPaths []string
	for _, b := range bins {
		if b.OS == "darwin" {
			darwinPaths = append(darwinPaths, b.Path)
		}
	}
	if len(darwinPaths) != 2 {
		t.Fatalf("expected 2 darwin entries, got %d", len(darwinPaths))
	}
	// Both darwin arches must point at the SAME on-disk fat binary, so the
	// content-addressed push gives them one shared blob digest (§3.1).
	if darwinPaths[0] != darwinPaths[1] {
		t.Errorf("darwin amd64/arm64 should share one path, got %q and %q", darwinPaths[0], darwinPaths[1])
	}
	if filepath.Base(filepath.Dir(darwinPaths[0])) != "pvtr-github-repo-scanner_darwin_all" {
		t.Errorf("darwin path should resolve under the _darwin_all dir, got %q", darwinPaths[0])
	}
}

func TestResolvePlatformBinaries_EntrypointFromExtraBinary(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	distDir := makeOriginalDistTree(t, arts)
	bins, err := resolvePlatformBinaries(arts, distDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, b := range bins {
		// The fixture's go-plugin entrypoint is "github-repo" (the build's
		// binary:), NOT the project name — that's exactly why we read
		// extra.Binary and not the artifact name/project.
		if b.Entrypoint != "github-repo" {
			t.Errorf("%s/%s: entrypoint = %q, want github-repo", b.OS, b.Arch, b.Entrypoint)
		}
	}
}

// The entrypoint must be byte-identical across ALL platforms (the hub requires
// it). In real GoReleaser output extra.Binary is ".exe"-free even on windows
// (only the artifact name carries .exe), so the loader — which reads
// extra.Binary — must produce the same entrypoint for every platform incl.
// windows. A regression that .exe-suffixed the windows entrypoint would break
// the hub's cross-child identity check.
func TestResolvePlatformBinaries_EntrypointIdenticalIncludingWindows(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	distDir := makeOriginalDistTree(t, arts)
	bins, err := resolvePlatformBinaries(arts, distDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, b := range bins {
		if b.Entrypoint != "github-repo" {
			t.Errorf("%s/%s entrypoint = %q, want github-repo (no .exe)", b.OS, b.Arch, b.Entrypoint)
		}
	}
}

func TestResolvePlatformBinaries_RelativePathsResolvedAgainstRepoRoot(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	distDir := makeOriginalDistTree(t, arts)
	bins, err := resolvePlatformBinaries(arts, distDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// distDir is <tmpdir>/dist; bins should resolve to <tmpdir>/dist/<id>/<bin>
	wantParent := distDir
	for _, b := range bins {
		if !filepath.IsAbs(b.Path) {
			t.Errorf("%s/%s path not absolute: %q", b.OS, b.Arch, b.Path)
		}
		if filepath.Dir(filepath.Dir(b.Path)) != wantParent {
			// path is <distDir>/<id>_<target>/<bin>
			t.Errorf("%s/%s path not under %s: %q", b.OS, b.Arch, wantParent, b.Path)
		}
	}
}

func TestResolvePlatformBinaries_MissingEntrypointErrors(t *testing.T) {
	arts := []goreleaserArtifact{
		{Name: "x", Path: "dist/x", GOOS: "linux", GOARCH: "amd64", Type: artifactTypeBinary},
	}
	// A missing entrypoint should error before any filesystem access.
	if _, err := resolvePlatformBinaries(arts, t.TempDir()); err == nil {
		t.Fatal("expected error for binary artifact with no extra.Binary, got nil")
	}
}

func TestResolvePlatformBinaries_SkipsNonBinaryTypes(t *testing.T) {
	arts := []goreleaserArtifact{
		{Name: "metadata.json", Path: "dist/metadata.json", Type: "Metadata"},
		{Name: "checksums.txt", Path: "dist/checksums.txt", Type: "Checksum"},
	}
	// No binaries to resolve; no filesystem access occurs.
	bins, err := resolvePlatformBinaries(arts, t.TempDir())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(bins) != 0 {
		t.Errorf("expected non-binary types skipped, got %d binaries", len(bins))
	}
}

func TestLoadGoReleaserBuild_MetadataVersion(t *testing.T) {
	// loadMetadata reads the committed real metadata.json fixture.
	m, err := loadMetadata(filepath.Join("testdata", "goreleaser-metadata.json"))
	if err != nil {
		t.Fatalf("loadMetadata: %v", err)
	}
	if m.Version == "" {
		t.Fatal("fixture metadata has no version")
	}
	if m.ProjectName != "pvtr-github-repo-scanner" {
		t.Errorf("project_name = %q", m.ProjectName)
	}
}

// TestLoadGoReleaserBuild_MovedDistDir exercises the "moved dist" resolution
// path (candidate 2 in resolveBinaryPath): the dist directory is named
// "downloaded" instead of "dist", and the artifacts.json paths still begin
// with "dist/". LoadGoReleaserBuild must strip the original leading "dist/"
// segment and resolve the binaries inside the renamed directory.
func TestLoadGoReleaserBuild_MovedDistDir(t *testing.T) {
	arts := loadFixtureArtifacts(t)
	// Build a moved dist layout: <tmpdir>/downloaded/<id>/<bin>
	distDir := makeMovedDistTree(t, arts, "downloaded")

	// Provide artifacts.json and metadata.json inside the moved dist directory
	// (LoadGoReleaserBuild reads them from distDir directly).
	artifactsData, err := os.ReadFile(filepath.Join("testdata", "goreleaser-artifacts.json"))
	if err != nil {
		t.Fatalf("reading artifacts fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "artifacts.json"), artifactsData, 0o644); err != nil {
		t.Fatalf("writing artifacts.json: %v", err)
	}
	metaData, err := os.ReadFile(filepath.Join("testdata", "goreleaser-metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "metadata.json"), metaData, 0o644); err != nil {
		t.Fatalf("writing metadata.json: %v", err)
	}

	version, bins, err := LoadGoReleaserBuild(distDir)
	if err != nil {
		t.Fatalf("LoadGoReleaserBuild on moved dist: %v", err)
	}
	if version == "" {
		t.Error("version empty")
	}

	// All 8 platforms (6 regular + 2 darwin re-expanded) must be found.
	want := []string{
		"linux/386", "linux/amd64", "linux/arm64",
		"windows/386", "windows/amd64", "windows/arm64",
		"darwin/amd64", "darwin/arm64",
	}
	got := map[string]string{}
	for _, b := range bins {
		got[b.OS+"/"+b.Arch] = b.Path
	}
	if len(bins) != len(want) {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("moved dist: expected %d platforms, got %d: %v", len(want), len(bins), keys)
	}
	for _, p := range want {
		if path, ok := got[p]; !ok {
			t.Errorf("moved dist: missing platform %s", p)
		} else if !strings.HasPrefix(path, distDir) {
			// All resolved paths must live inside the moved dist directory.
			t.Errorf("moved dist: path for %s not under distDir %q: %q", p, distDir, path)
		}
	}
}

// TestLoadGoReleaserBuild_NotFoundErrorMentionsBothCandidates verifies that
// when neither candidate path exists, the error message names both tried paths
// so the user knows exactly where the loader looked.
func TestLoadGoReleaserBuild_NotFoundErrorMentionsBothCandidates(t *testing.T) {
	root := t.TempDir()
	distDir := filepath.Join(root, "emptydist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir distDir: %v", err)
	}

	// Provide artifacts.json and metadata.json with one binary entry but no
	// actual binary file on disk.
	arts := []goreleaserArtifact{{
		Name:   "github-repo",
		Path:   "dist/pvtr-github-repo-scanner_linux_amd64_v1/github-repo",
		GOOS:   "linux",
		GOARCH: "amd64",
		Type:   artifactTypeBinary,
	}}
	arts[0].Extra.Binary = "github-repo"

	artsData, err := json.Marshal(arts)
	if err != nil {
		t.Fatalf("marshal arts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "artifacts.json"), artsData, 0o644); err != nil {
		t.Fatalf("write artifacts.json: %v", err)
	}

	metaData := []byte(`{"version":"v1.0.0","project_name":"test"}`)
	if err := os.WriteFile(filepath.Join(distDir, "metadata.json"), metaData, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	_, _, err = LoadGoReleaserBuild(distDir)
	if err == nil {
		t.Fatal("expected error when binary files are missing, got nil")
	}

	// The error must mention both candidate paths so the user knows where the
	// loader looked.
	cand1, cand2 := artifactCandidates(distDir, arts[0].Path)
	if !strings.Contains(err.Error(), cand1) {
		t.Errorf("error %q does not mention candidate 1 %q", err.Error(), cand1)
	}
	if !strings.Contains(err.Error(), cand2) {
		t.Errorf("error %q does not mention candidate 2 %q", err.Error(), cand2)
	}
}

// TestStripFirstSegment verifies the slash-normalized first-segment stripping
// used by the moved-dist path resolution. Windows-style backslash paths are
// intentionally tested via filepath.FromSlash to confirm ToSlash normalization
// happens before the split.
func TestStripFirstSegment(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"dist/foo/bar", "foo/bar"},
		{"dist/x", "x"},
		{"nodash", "nodash"}, // no separator: path as-is
		{"a/b/c/d", "b/c/d"},
		{"", ""},
	}
	for _, tc := range cases {
		got := stripFirstSegment(tc.in)
		if got != tc.want {
			t.Errorf("stripFirstSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
