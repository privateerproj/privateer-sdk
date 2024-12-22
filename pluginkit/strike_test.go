package pluginkit

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

var executeMovementTests = []struct {
	testName        string
	expectedPass    bool
	expectedMessage string
	strikeResult    *StrikeResult
	movementResult  MovementResult
}{
	{
		testName:        "First movement passed",
		expectedPass:    true,
		expectedMessage: "Movement successful",
		strikeResult: &StrikeResult{
			Message:   "No previous movements",
			Movements: make(map[string]MovementResult),
		},
		movementResult: MovementResult{
			Passed:  true,
			Message: "Movement successful",
		},
	},
	{
		testName:        "First movement failed",
		expectedPass:    false,
		expectedMessage: "Movement failed",
		strikeResult: &StrikeResult{
			Message:   "No previous movements",
			Movements: make(map[string]MovementResult),
		},
		movementResult: MovementResult{
			Passed:  false,
			Message: "Movement failed",
		},
	},
	{
		testName:        "Previous movement passed-current movement passed",
		expectedPass:    true,
		expectedMessage: "Movement failed",
		strikeResult: &StrikeResult{
			Passed:  true,
			Message: "Previous movement passed",
			Movements: map[string]MovementResult{
				"movement1": {
					Passed:  true,
					Message: "Movement successful",
				},
			},
		},
		movementResult: MovementResult{
			Passed:  true,
			Message: "Movement failed",
		},
	},
	{
		testName:        "Previous movement failed-current movement passed",
		expectedPass:    false,
		expectedMessage: "Previous movement failed",
		strikeResult: &StrikeResult{
			Passed:  false,
			Message: "Previous movement failed",
			Movements: map[string]MovementResult{
				"movement1": {
					Passed:  false,
					Message: "Movement failed",
				},
			},
		},
		movementResult: MovementResult{
			Passed:  true,
			Message: "Movement successful",
		},
	},
	{
		testName:        "Previous movement passed-current movement failed",
		expectedPass:    false,
		expectedMessage: "Movement failed",
		strikeResult: &StrikeResult{
			Passed:  true,
			Message: "Previous movement passed",
			Movements: map[string]MovementResult{
				"movement1": {
					Passed:  true,
					Message: "Movement successful",
				},
			},
		},
		movementResult: MovementResult{
			Passed:  false,
			Message: "Movement failed",
		},
	},
	{
		testName:        "Previous movement failed-current movement failed",
		expectedPass:    false,
		expectedMessage: "Previous movement failed",
		strikeResult: &StrikeResult{
			Passed:  false,
			Message: "Previous movement failed",
			Movements: map[string]MovementResult{
				"movement1": {
					Passed:  false,
					Message: "Movement failed",
				},
			},
		},
		movementResult: MovementResult{
			Passed:  false,
			Message: "Movement failed",
		},
	},
}

func TestExecuteMovement(t *testing.T) {
	for _, tt := range executeMovementTests {
		t.Run(tt.testName, func(t *testing.T) {
			tt.strikeResult.ExecuteMovement(func() MovementResult {
				return tt.movementResult
			})

			if tt.expectedPass != tt.strikeResult.Passed {
				t.Errorf("strikeResult.Passed = %v, Expected: %v", tt.strikeResult.Passed, tt.expectedPass)
			}
			if tt.expectedMessage != tt.strikeResult.Message {
				t.Errorf("strikeResult.Message = %v, Expected: %v", tt.strikeResult.Message, tt.expectedMessage)
			}
		})
	}
}

func TestExecuteInvasiveMovement(t *testing.T) {
	for _, tt := range executeMovementTests {
		for _, invasive := range []bool{false, true} {
			// Clone the strikeResult to avoid side effects
			result := &StrikeResult{
				Passed:    tt.strikeResult.Passed,
				Message:   tt.strikeResult.Message,
				Movements: make(map[string]MovementResult),
			}
			for k, v := range tt.strikeResult.Movements {
				result.Movements[k] = v
			}

			t.Run(fmt.Sprintf("%s-invasive=%v)", tt.testName, invasive), func(t *testing.T) {
				viper.Set("invasive", invasive)

				// Simulate a movement function execution
				result.ExecuteInvasiveMovement(func() MovementResult {
					return tt.movementResult
				})

				if invasive {
					if tt.expectedPass != result.Passed {
						t.Errorf("strikeResult.Passed = %v, Expected: %v", result.Passed, tt.expectedPass)
					}
					if tt.expectedMessage != result.Message {
						t.Errorf("strikeResult.Message = %v, Expected: %v", result.Message, tt.expectedMessage)
					}
				} else {
					if tt.strikeResult.Passed != result.Passed {
						t.Errorf("strikeResult.Passed = %v, Expected: %v", result.Passed, tt.strikeResult.Passed)
					}
					if tt.strikeResult.Message != result.Message {
						t.Errorf("strikeResult.Message = %v, Expected: %v", result.Message, tt.strikeResult.Message)
					}
				}
			})
		}
	}
}
