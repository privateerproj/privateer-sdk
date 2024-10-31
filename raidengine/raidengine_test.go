package raidengine

import (
	"testing"
)

func TestExecuteMovement(t *testing.T) {

	var tests = []struct {
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

	for _, tt := range tests {
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
