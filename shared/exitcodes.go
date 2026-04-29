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
