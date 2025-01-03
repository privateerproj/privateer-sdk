package pluginkit

type Test func() *TestResult

// TestResult is a struct that contains the results of a single step within a testSet
type TestResult struct {
	Passed      bool               // Passed is true if the test passed
	Description string             // Description is a human-readable description of the test
	Message     string             // Message is a human-readable description of the test result
	Function    string             // Function is the name of the code that was executed
	Value       interface{}        // Value is the object that was returned during the test
	Changes     map[string]*Change // Changes is a slice of changes that were made during the test
}

func (t *TestResult) Pass(message string, value interface{}) {
	t.Passed = true
	t.Message = message
	t.Value = value
}

func (t *TestResult) Fail(message string, value interface{}) {
	t.Passed = false
	t.Message = message
	t.Value = value
}
