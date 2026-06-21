package install

import (
	"fmt"
	"regexp"
	"strings"
)

// validNameSegmentRegex bounds a namespace, plugin id, or binary filename to a safe,
// path-component-valid shape.
var validNameSegmentRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// parseCoordinate splits a "<namespace>/<plugin_id>[@<version>]" argument.
// grc.store has no default namespace, so a bare name (no '/') is an error
func parseCoordinate(arg string) (namespace, pluginId, version string, err error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		err = fmt.Errorf("plugin coordinate must not be empty")
		return
	}
	coord, ver, _ := strings.Cut(arg, "@") // version is optional
	coord = strings.TrimSpace(coord)
	version = normalizeVersion(strings.TrimSpace(ver))

	namespace, pluginId, ok := strings.Cut(coord, "/")
	if !ok {
		err = fmt.Errorf("%q is not a grc.store coordinate — use <namespace>/<plugin_id> (e.g. ossf/pvtr-github-repo)", coord)
		return
	}
	if !validNameSegmentRegex.MatchString(namespace) {
		err = fmt.Errorf("invalid namespace %q", namespace)
		return
	}
	if !validNameSegmentRegex.MatchString(pluginId) {
		err = fmt.Errorf("invalid plugin id %q", pluginId)
		return
	}
	return
}

// normalizeVersion strips a leading "v" before a digit so "v1.4.0" and "1.4.0"
// resolve identically against the hub and manifest.
func normalizeVersion(v string) string {
	if len(v) >= 2 && v[0] == 'v' && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}
