package pluginkit

type Movement func() *MovementResult

// MovementResult is a struct that contains the results of a single step within a strike
type MovementResult struct {
	Passed      bool               // Passed is true if the test passed
	Description string             // Description is a human-readable description of the test
	Message     string             // Message is a human-readable description of the test result
	Function    string             // Function is the name of the code that was executed
	Value       interface{}        // Value is the object that was returned during the movement
	Changes     map[string]*Change // Changes is a slice of changes that were made during the movement
}
