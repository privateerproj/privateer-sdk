package pluginkit

import (
	"errors"
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

	change := NewChange(targetName, description, targetObject, goodApplyFunc, goodRevertFunc)

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

func TestChangeManager_Add(t *testing.T) {
	cm := &ChangeManager{}
	changeName := "test-change"
	change := pendingChange()

	cm.Add(changeName, change)

	if cm.Changes == nil {
		t.Error("Changes map should be initialized")
	}
	if _, exists := cm.Changes[changeName]; !exists {
		t.Error("Change should be added to the Changes map")
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

func TestChangeManager_BadState(t *testing.T) {
	cm := &ChangeManager{
		Allowed: true,
		Changes: map[string]*Change{
			"bad-change": {
				TargetName:  "test",
				Description: "bad change",
				applyFunc:   badApplyFunc,
				revertFunc:  goodRevertFunc,
			},
		},
	}

	success, _ := cm.Apply("bad-change", "test-target", "test-input")
	if success {
		t.Error("Apply should fail with bad apply function")
	}
	if !cm.CorruptedState {
		t.Error("ChangeManager should be in bad state after failed apply")
	}
}
