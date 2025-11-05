package pluginkit

import (
	"errors"
	"strings"
	"testing"
)

var (
	// Functions
	goodApplyFunc = func(interface{}) (interface{}, error) {
		return "applied_result", nil
	}
	goodRevertFunc = func(interface{}) error {
		return nil
	}
	badApplyFunc = func(interface{}) (interface{}, error) {
		return nil, errors.New("apply error")
	}
	badRevertFunc = func(interface{}) error {
		return errors.New("revert error")
	}
)

func pendingChange() Change {
	return Change{
		TargetName:  "pendingChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
	}
}

func appliedNotRevertedChange() Change {
	return Change{
		TargetName:  "appliedNotRevertedChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
		Applied:     true,
	}
}
func badApplyChange() Change {
	return Change{
		TargetName:  "badApplyChange",
		Description: "description placeholder",
		applyFunc:   badApplyFunc,
		revertFunc:  goodRevertFunc,
	}
}
func badRevertChange() Change {
	return Change{
		TargetName:  "badRevertChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  badRevertFunc,
	}
}
func noApplyChange() Change {
	return Change{
		TargetName:  "noApplyChange",
		Description: "description placeholder",
		revertFunc:  goodRevertFunc,
	}
}
func noFuncsChange() Change {
	return Change{
		TargetName:  "noFuncsChange",
		Description: "description placeholder",
	}
}

// Test functions for Change
func TestChange_Apply(t *testing.T) {
	tests := []struct {
		name            string
		change          Change
		targetName      string
		changeInput     interface{}
		expectedSuccess bool
		expectedApplied bool
	}{
		{
			name:            "Successful apply",
			change:          pendingChange(),
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: true,
			expectedApplied: true,
		},
		{
			name:            "Apply with bad function",
			change:          badApplyChange(),
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: false,
			expectedApplied: false,
		},
		{
			name:            "Apply already applied change",
			change:          appliedNotRevertedChange(),
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: true,
			expectedApplied: true,
		},
		{
			name:            "Apply missing apply function",
			change:          noApplyChange(),
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: false,
			expectedApplied: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			success, _ := tt.change.apply(tt.targetName, tt.changeInput)
			if success != tt.expectedSuccess {
				t.Errorf("apply() success = %v, expected %v", success, tt.expectedSuccess)
			}
			if tt.change.Applied != tt.expectedApplied {
				t.Errorf("Applied = %v, expected %v", tt.change.Applied, tt.expectedApplied)
			}
		})
	}
}

func TestChange_Revert(t *testing.T) {
	tests := []struct {
		name             string
		change           Change
		data             interface{}
		expectedReverted bool
		expectedBadState bool
	}{
		{
			name:             "Successful revert",
			change:           appliedNotRevertedChange(),
			data:             "test-data",
			expectedReverted: true,
			expectedBadState: false,
		},
		{
			name:             "Revert not applied change",
			change:           pendingChange(),
			data:             "test-data",
			expectedReverted: false,
			expectedBadState: false,
		},
		{
			name:             "Revert with bad function",
			change:           badRevertChange(),
			data:             "test-data",
			expectedReverted: false,
			expectedBadState: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make sure the change is applied first for revert tests
			if tt.change.TargetName == "badRevertChange" {
				tt.change.Applied = true
			}

			tt.change.revert(tt.data)

			if tt.change.Reverted != tt.expectedReverted {
				t.Errorf("Reverted = %v, expected %v", tt.change.Reverted, tt.expectedReverted)
			}
			if tt.change.CorruptedState != tt.expectedBadState {
				t.Errorf("CorruptedState = %v, expected %v", tt.change.CorruptedState, tt.expectedBadState)
			}
		})
	}
}

func TestNewChange(t *testing.T) {
	targetName := "test-target"
	description := "test description"
	targetObject := "test-object"

	change := Change{
		TargetName:   targetName,
		Description:  description,
		TargetObject: targetObject,
	}

	change.AddFunctions(goodApplyFunc, goodRevertFunc)

	if change.TargetName != targetName {
		t.Errorf("TargetName = %v, expected %v", change.TargetName, targetName)
	}
	if change.Description != description {
		t.Errorf("Description = %v, expected %v", change.Description, description)
	}
	if change.TargetObject != targetObject {
		t.Errorf("TargetObject = %v, expected %v", change.TargetObject, targetObject)
	}
}

// Test functions for ChangeManager
func TestChangeManager_Allow(t *testing.T) {
	cm := &ChangeManager{}
	if cm.Allowed {
		t.Error("ChangeManager should not be allowed by default")
	}

	cm.Allow()
	if !cm.Allowed {
		t.Error("ChangeManager should be allowed after calling Allow()")
	}
}

func TestChangeManager_AddChange(t *testing.T) {
	cm := &ChangeManager{}
	changeName := "test-change"
	change := pendingChange()

	cm.AddChange(changeName, change)

	if cm.Changes == nil {
		t.Error("Changes map should be initialized")
	}
	if _, exists := cm.Changes[changeName]; !exists {
		t.Error("Change should be added to the Changes map")
	}
}

func TestChangeManager_AddFunctions(t *testing.T) {
	change := noFuncsChange()
	if change.applyFunc != nil || change.revertFunc != nil {
		t.Error("Test is malformed: expected nil functions but got non-nil")
	}

	change.AddFunctions(goodApplyFunc, goodRevertFunc)
	if change.applyFunc == nil || change.revertFunc == nil {
		t.Error("Change functions should be updated")
	}
}

func TestChangeManager_Apply(t *testing.T) {
	tests := []struct {
		name            string
		allowed         bool
		changeName      string
		targetName      string
		changeInput     interface{}
		expectedSuccess bool
	}{
		{
			name:            "Apply when allowed",
			allowed:         true,
			changeName:      "test-change",
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: true,
		},
		{
			name:            "Apply when not allowed",
			allowed:         false,
			changeName:      "test-change",
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: false,
		},
		{
			name:            "Apply non-existent change",
			allowed:         true,
			changeName:      "non-existent",
			targetName:      "test-target",
			changeInput:     "test-input",
			expectedSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ChangeManager{
				Allowed: tt.allowed,
				Changes: map[string]*Change{
					"test-change": {
						TargetName:  "test",
						Description: "test change",
						applyFunc:   goodApplyFunc,
						revertFunc:  goodRevertFunc,
					},
				},
			}

			success, _ := cm.Apply(tt.changeName, tt.targetName, tt.changeInput)
			if success != tt.expectedSuccess {
				t.Errorf("Apply() success = %v, expected %v", success, tt.expectedSuccess)
			}
		})
	}
}

func TestChangeManager_Revert(t *testing.T) {
	cm := &ChangeManager{
		Changes: map[string]*Change{
			"test-change": {
				TargetName:   "test",
				Description:  "test change",
				applyFunc:    goodApplyFunc,
				revertFunc:   goodRevertFunc,
				Applied:      true,
				TargetObject: "test-object",
			},
		},
	}

	cm.Revert("test-change")

	change := cm.Changes["test-change"]
	if !change.Reverted {
		t.Error("Change should be reverted")
	}
}

func TestChangeManager_RevertAll(t *testing.T) {
	t.Run("Revert All Changes", func(t *testing.T) {
		cm := &ChangeManager{
			Changes: map[string]*Change{
				"change1": {
					TargetName:   "target1",
					Description:  "change 1",
					applyFunc:    goodApplyFunc,
					revertFunc:   goodRevertFunc,
					Applied:      true,
					TargetObject: "object1",
				},
				"change2": {
					TargetName:   "target2",
					Description:  "change 2",
					applyFunc:    goodApplyFunc,
					revertFunc:   goodRevertFunc,
					Applied:      true,
					TargetObject: "object2",
				},
			},
		}

		cm.RevertAll()

		for name, change := range cm.Changes {
			if !change.Reverted {
				t.Errorf("Change %s should be reverted", name)
			}
		}
	})

	t.Run("Revert All With Bad Revert Function", func(t *testing.T) {
		cm := &ChangeManager{
			Changes: map[string]*Change{
				"bad-change": {
					TargetName:   "target",
					Description:  "bad change",
					applyFunc:    goodApplyFunc,
					revertFunc:   badRevertFunc,
					Applied:      true,
					TargetObject: "object",
				},
			},
		}

		cm.RevertAll()

		change := cm.Changes["bad-change"]
		if change.Reverted {
			t.Error("Change should not be reverted with bad revert function")
		}
		if !change.CorruptedState {
			t.Error("Change should be in corrupted state")
		}
		if !cm.CorruptedState {
			t.Error("ChangeManager should be in corrupted state")
		}
	})

	t.Run("Revert All Empty Changes", func(t *testing.T) {
		cm := &ChangeManager{
			Changes: map[string]*Change{},
		}

		// Should not panic
		cm.RevertAll()
	})
}

func TestChange_Precheck(t *testing.T) {
	t.Run("Valid Change", func(t *testing.T) {
		change := Change{
			TargetName:  "test-target",
			Description: "test description",
			applyFunc:   goodApplyFunc,
			revertFunc:  goodRevertFunc,
		}

		err := change.precheck()
		if err != nil {
			t.Errorf("Expected no error for valid change, got: %v", err)
		}
	})

	t.Run("Missing Apply Function", func(t *testing.T) {
		change := Change{
			TargetName:  "test-target",
			Description: "test description",
			revertFunc:  goodRevertFunc,
		}

		err := change.precheck()
		if err == nil {
			t.Error("Expected error for missing apply function")
		}
		if !strings.Contains(err.Error(), "applyFunc and revertFunc must be defined") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Missing Revert Function", func(t *testing.T) {
		change := Change{
			TargetName:  "test-target",
			Description: "test description",
			applyFunc:   goodApplyFunc,
		}

		err := change.precheck()
		if err == nil {
			t.Error("Expected error for missing revert function")
		}
		if !strings.Contains(err.Error(), "applyFunc and revertFunc must be defined") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Missing Target Name", func(t *testing.T) {
		change := Change{
			Description: "test description",
			applyFunc:   goodApplyFunc,
			revertFunc:  goodRevertFunc,
		}

		err := change.precheck()
		if err == nil {
			t.Error("Expected error for missing target name")
		}
		if !strings.Contains(err.Error(), "change must have a TargetName and Description defined") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Missing Description", func(t *testing.T) {
		change := Change{
			TargetName: "test-target",
			applyFunc:  goodApplyFunc,
			revertFunc: goodRevertFunc,
		}

		err := change.precheck()
		if err == nil {
			t.Error("Expected error for missing description")
		}
		if !strings.Contains(err.Error(), "change must have a TargetName and Description defined") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Change With Previous Error", func(t *testing.T) {
		change := Change{
			TargetName:  "test-target",
			Description: "test description",
			applyFunc:   goodApplyFunc,
			revertFunc:  goodRevertFunc,
			Error:       errors.New("previous error"),
		}

		err := change.precheck()
		if err == nil {
			t.Error("Expected error for change with previous error")
		}
		if !strings.Contains(err.Error(), "change has a previous error") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestChangeManager_RevertNonExistent(t *testing.T) {
	cm := &ChangeManager{
		Changes: map[string]*Change{},
	}

	// Should not panic when reverting non-existent change
	cm.Revert("non-existent-change")
}
