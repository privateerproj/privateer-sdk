package shared

// Canonical exit codes. Defined here so command/ and pluginkit/ can both
// reference them without an import cycle.
const (
	TestPass = iota
	TestFail
	Aborted
	InternalError
	BadUsage
	NoTests
	exitCodeCount // add new codes above this line, and give each an exitSeverity entry
)

// Across multi-plugin runs the most severe outcome wins. Every exit code needs
// an entry here, because a missing one scores 0 and merges as if it were
// TestPass. NoTests ranks just above TestPass, so a plugin that returns NoTests
// never reads as a pass, while a real TestFail still wins over it.
var exitSeverity = map[int]int{
	TestPass:      0,
	NoTests:       1,
	TestFail:      2,
	Aborted:       3,
	BadUsage:      4,
	InternalError: 5,
}

// MergeExitCode returns whichever of prev and next is the more severe outcome,
// keeping prev on a tie.
func MergeExitCode(prev, next int) int {
	if exitSeverity[next] > exitSeverity[prev] {
		return next
	}
	return prev
}
