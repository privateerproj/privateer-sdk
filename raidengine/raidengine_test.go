package raidengine

import (
	"testing"

	"github.com/spf13/viper"
)

var executeMovementTests = []struct {
	testName       string
	expectedPass   bool
	strikeResult   *StrikeResult
	movementResult MovementResult
}{
	{
		testName:     "First movement passed",
		expectedPass: true,
		strikeResult: &StrikeResult{
			Movements: make(map[string]MovementResult),
		},
		movementResult: MovementResult{
			Passed:  true,
			Message: "Movement successful",
		},
	},
	{
		testName:     "First movement failed",
		expectedPass: false,
		strikeResult: &StrikeResult{
			Movements: make(map[string]MovementResult),
		},
		movementResult: MovementResult{
			Passed:  false,
			Message: "Movement failed",
		},
	},
	{
		testName:     "Previous movement passed current movement passed",
		expectedPass: true,
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
		testName:     "Previous movement failed current movement passed",
		expectedPass: false,
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
		testName:     "Previous movement passed current movement failed",
		expectedPass: false,
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
		testName:     "Previous movement failed current movement failed",
		expectedPass: false,
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
			ExecuteMovement(tt.strikeResult, func() MovementResult {
				return tt.movementResult
			})

			if tt.expectedPass != tt.strikeResult.Passed {
				t.Errorf("strikeResult.Passed = %v, Expected: %v", tt.strikeResult.Passed, tt.expectedPass)
			}
		})
	}
}

func TestExecuteInvasiveMovement(t *testing.T) {
	for _, tt := range executeMovementTests {
		previousResult := tt.strikeResult
		for _, invasive := range []bool{true, false} {
			t.Logf("Invasive: %v", invasive)
			t.Logf("Previous Result: %v", previousResult)
			t.Run(tt.testName, func(t *testing.T) {
				viper.Set("invasive", true)
				ExecuteInvasiveMovement(tt.strikeResult, func() MovementResult {
					return tt.movementResult
				})

				if !invasive {
					if tt.expectedPass != tt.strikeResult.Passed {
						t.Errorf("strikeResult.Passed = %v, Expected: %v", tt.strikeResult.Passed, tt.expectedPass)
					}
				}
				if invasive {
					if previousResult.Passed != tt.strikeResult.Passed {
						t.Errorf("strikeResult.Passed = %v, Expected: %v", tt.strikeResult.Passed, previousResult.Passed)
					}
					if previousResult.Message != tt.strikeResult.Message {
						t.Errorf("strikeResult.Message = %v, Expected: %v", tt.strikeResult.Message, previousResult.Message)
					}
					if len(previousResult.Movements) != len(tt.strikeResult.Movements) {
						t.Errorf("strikeResult.Movements = %v, Expected: %v", tt.strikeResult.Movements, previousResult.Movements)
					}
				}
			})
		}
	}
}
