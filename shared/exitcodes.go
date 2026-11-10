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
)

// ExitCodeName returns the name of a canonical exit code, or "" when the code
// is outside the canonical set.
func ExitCodeName(code int) string {
	switch code {
	case TestPass:
		return "TestPass"
	case TestFail:
		return "TestFail"
	case Aborted:
		return "Aborted"
	case InternalError:
		return "InternalError"
	case BadUsage:
		return "BadUsage"
	case NoTests:
		return "NoTests"
	}
	return ""
}
