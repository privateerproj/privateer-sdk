package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoReleaser emits dist/artifacts.json (every artifact) and dist/metadata.json
// (version/tag/project). The publisher assembles the OCI plugin index from the
// raw build BINARIES in artifacts.json — NOT the tar.gz archives, which carry
// no per-platform OCI structure. These types decode exactly the fields the
// assembler needs from the real GoReleaser v2 output (grounded against a real
// `goreleaser build` of pvtr-github-repo-scanner; see testdata/).

// artifactTypeBinary is the GoReleaser artifact "type" for a built binary.
// A darwin universal binary (universal_binaries: replace: true) also appears
// as type "Binary" but with goarch "all" and extra.Replaces true — the per-arch
// darwin entries are removed from artifacts.json in that case.
const artifactTypeBinary = "Binary"

// darwinUniversalArch is the goarch GoReleaser assigns to a darwin universal
// (fat) binary. It is re-expanded by the index assembler into one descriptor
// per real architecture, all pointing at the single fat-binary blob.
const darwinUniversalArch = "all"

// goreleaserArtifact is one entry in dist/artifacts.json. Only the fields the
// assembler consumes are decoded.
type goreleaserArtifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Type   string `json:"type"`
	Extra  struct {
		// Binary is the go-plugin entrypoint name (the build's `binary:`),
		// which is what the plugin must be installed as. It can differ from
		// the project/repo name.
		Binary string `json:"Binary"`
		// Replaces is true on the darwin universal entry produced by
		// universal_binaries: replace: true.
		Replaces bool `json:"Replaces"`
	} `json:"extra"`
}

// goreleaserMetadata decodes the fields of dist/metadata.json the assembler
// needs: the release version (and the project name as a fallback identifier).
type goreleaserMetadata struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	ProjectName string `json:"project_name"`
}

// PlatformBinary is one resolved (os, arch) -> binary mapping the index
// assembler turns into a child manifest. Universal darwin binaries are already
// re-expanded here: the single fat binary yields two PlatformBinaries
// (darwin/amd64, darwin/arm64) sharing the same Path, so a later content-addressed
// push gives them the same blob digest.
type PlatformBinary struct {
	OS         string // GOOS, e.g. "linux", "darwin", "windows"
	Arch       string // GOARCH, e.g. "amd64", "arm64", "386"
	Path       string // absolute path to the binary on disk
	Entrypoint string // go-plugin entrypoint name (extra.Binary), .exe-suffixed on windows
}

// LoadGoReleaserBuild reads dist/artifacts.json + dist/metadata.json from a
// GoReleaser dist directory and returns the release version and the resolved
// per-platform binaries (darwin universal already re-expanded). Paths in
// artifacts.json are relative to the repo root GoReleaser ran in
// (e.g. "dist/<id>_linux_amd64_v1/<bin>"). When the dist directory has been
// renamed or downloaded to a different location, the path-resolution logic
// tries two candidates: the original repo-root-relative layout, and a
// stripped layout inside distDir (see resolvePlatformBinaries).
func LoadGoReleaserBuild(distDir string) (version string, bins []PlatformBinary, err error) {
	meta, err := loadMetadata(filepath.Join(distDir, "metadata.json"))
	if err != nil {
		return "", nil, err
	}
	version = meta.Version
	if version == "" {
		return "", nil, fmt.Errorf("metadata.json has no version")
	}

	arts, err := loadArtifacts(filepath.Join(distDir, "artifacts.json"))
	if err != nil {
		return "", nil, err
	}

	bins, err = resolvePlatformBinaries(arts, distDir)
	if err != nil {
		return "", nil, err
	}
	if len(bins) == 0 {
		return "", nil, fmt.Errorf("no binary artifacts found in %s", filepath.Join(distDir, "artifacts.json"))
	}
	return version, bins, nil
}

// stripFirstSegment removes the leading path segment from a slash-normalized
// path. For example, "dist/foo/bar" becomes "foo/bar". If there is no slash
// separator, the path is returned as-is. This lets the moved-dist resolution
// candidate strip the original "dist/" prefix recorded in artifacts.json so
// that paths can be resolved inside a renamed dist directory.
func stripFirstSegment(p string) string {
	// Normalize to forward slashes so the split works on Windows paths too
	// (artifacts.json always uses forward slashes; filepath.ToSlash is a no-op
	// on platforms that already use forward slashes).
	slashed := filepath.ToSlash(p)
	if _, rest, found := strings.Cut(slashed, "/"); found {
		return rest
	}
	return slashed
}

// artifactCandidates returns the two candidate absolute paths for a relative
// artifact path recorded in artifacts.json, without checking whether they
// exist. Callers that need existence checking (e.g. resolveBinaryPath) call
// os.Stat on each in order.
//
//  1. Original layout: filepath.Join(filepath.Dir(distDir), artifactPath) —
//     works when distDir still sits where GoReleaser ran (e.g. ./dist).
//  2. Moved-dist layout: filepath.Join(distDir, stripFirstSegment(artifactPath)) —
//     works when the dist directory was renamed or downloaded elsewhere
//     (e.g. ./downloaded) and the inner sub-directories still match.
func artifactCandidates(distDir, artifactPath string) (cand1, cand2 string) {
	// filepath.FromSlash is a no-op on POSIX; on Windows it converts the
	// forward slashes that artifacts.json always uses into backslashes.
	cand1 = filepath.Join(filepath.Dir(distDir), filepath.FromSlash(artifactPath))
	cand2 = filepath.Join(distDir, filepath.FromSlash(stripFirstSegment(artifactPath)))
	return cand1, cand2
}

// resolveBinaryPath resolves a relative artifact path to an absolute one using
// two candidate strategies (see artifactCandidates), returning the first that
// exists on disk. Absolute paths are returned unchanged without a stat check.
//
// If neither candidate exists, an error is returned listing both paths and
// advising the caller to pass --dist pointing at the downloaded dist directory.
func resolveBinaryPath(distDir, artifactPath, osArch string) (string, error) {
	if filepath.IsAbs(artifactPath) {
		return artifactPath, nil
	}

	cand1, cand2 := artifactCandidates(distDir, artifactPath)

	// Candidate 1: original layout — distDir's parent + the recorded path.
	// GoReleaser records paths like "dist/<id>_linux_amd64_v1/<bin>", so the
	// parent of distDir is the repo root the relative path is anchored to.
	if _, err := os.Stat(cand1); err == nil {
		return cand1, nil
	}

	// Candidate 2: moved-dist layout — distDir + path with its first segment
	// stripped. When the dist directory was downloaded to a different location
	// (common CI pattern: `pvtr publish --dist ./downloaded`), the original
	// "dist/" prefix in the recorded path must be replaced with distDir.
	if _, err := os.Stat(cand2); err == nil {
		return cand2, nil
	}

	return "", fmt.Errorf(
		"binary for %s not found: tried %q and %q (artifacts.json records paths relative to the original goreleaser run; pass --dist pointing at the downloaded dist directory)",
		osArch, cand1, cand2,
	)
}

// resolvePlatformBinaries filters artifacts to binaries and re-expands the
// darwin universal entry into per-arch PlatformBinaries. Extracted from
// LoadGoReleaserBuild so it is unit-testable without touching the filesystem
// for the artifacts list itself.
//
// distDir is the GoReleaser dist directory passed by the caller. Relative
// artifact paths are resolved via resolveBinaryPath, which tries the original
// repo-root-relative layout first, then a moved-dist layout, so the function
// survives CI patterns where the dist directory is downloaded under a different
// name (see Task 1F in the remediation plan).
func resolvePlatformBinaries(arts []goreleaserArtifact, distDir string) ([]PlatformBinary, error) {
	var out []PlatformBinary
	for _, a := range arts {
		if a.Type != artifactTypeBinary {
			continue
		}
		entrypoint := a.Extra.Binary
		if entrypoint == "" {
			return nil, fmt.Errorf("binary artifact %q has no extra.Binary (entrypoint) name", a.Name)
		}

		if a.GOOS == "darwin" && a.GOARCH == darwinUniversalArch {
			// Re-expand the universal (fat) binary into the two real arches it
			// contains. Both point at the same on-disk file, so the OCI index
			// gets two descriptors over one blob digest (the §3.1 contract).
			absPath, err := resolveBinaryPath(distDir, a.Path, "darwin/all")
			if err != nil {
				return nil, err
			}
			for _, arch := range []string{"amd64", "arm64"} {
				out = append(out, PlatformBinary{
					OS:         "darwin",
					Arch:       arch,
					Path:       absPath,
					Entrypoint: entrypoint,
				})
			}
			continue
		}

		osArch := a.GOOS + "/" + a.GOARCH
		absPath, err := resolveBinaryPath(distDir, a.Path, osArch)
		if err != nil {
			return nil, err
		}
		out = append(out, PlatformBinary{
			OS:         a.GOOS,
			Arch:       a.GOARCH,
			Path:       absPath,
			Entrypoint: entrypoint,
		})
	}
	return out, nil
}

func loadArtifacts(path string) ([]goreleaserArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var arts []goreleaserArtifact
	if err := json.Unmarshal(data, &arts); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return arts, nil
}

func loadMetadata(path string) (*goreleaserMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m goreleaserMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}
