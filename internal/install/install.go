// Package install installs Privateer plugins — from grc.store (pulled and
// verified end-to-end) or from a local binary path.
package install

// The command package owns the CLI wiring and calls Local / FromStore
// The install logic and its tests live here so command/ stays a thin
// layer over the internal packages.
