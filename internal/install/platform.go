package install

import (
	"fmt"
	"runtime"
	"strings"
)

// InferGitHubReleaseBase returns the base URL for a GitHub release.
// source can be a full URL (https://github.com/owner/repo) or "owner/repo" format.
// version can be with or without a "v" prefix — it will be normalized to include one.
func InferGitHubReleaseBase(source, version string) string {
	source = strings.TrimSuffix(source, "/")
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return fmt.Sprintf("%s/releases/download/%s", source, version)
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s", source, version)
}

// InferArtifactFilename returns the expected artifact filename for the current platform.
// It follows the GoReleaser naming convention: {name}_{OS}_{arch}.{ext}
// Darwin uses "all" for universal binaries (matching GoReleaser's universal_binaries config).
func InferArtifactFilename(name string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Match GoReleaser conventions
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "i386"
	}
	osName = strings.ToUpper(osName[:1]) + osName[1:] // capitalize first letter

	// GoReleaser universal binaries use "all" for Darwin
	if runtime.GOOS == "darwin" {
		arch = "all"
	}

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s.%s", name, osName, arch, ext)
}
