package install

import (
	"fmt"
	"runtime"
	"strings"
)

// InferGitHubReleaseBase returns the base URL for a GitHub release.
// source should be in "owner/repo" format, version should be a tag like "v1.0.0".
func InferGitHubReleaseBase(source, version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s", source, version)
}

// InferArtifactFilename returns the expected artifact filename for the current platform.
// It follows the GoReleaser naming convention: {name}_{OS}_{arch}.tar.gz
func InferArtifactFilename(name string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Match GoReleaser conventions
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "i386"
	}
	os = strings.ToTitle(os[:1]) + os[1:] // capitalize first letter

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s.%s", name, os, arch, ext)
}
