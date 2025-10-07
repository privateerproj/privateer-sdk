package pluginkit

import (
	"fmt"
)

// Prepared function to apply the change
type ApplyFunc func(interface{}) (interface{}, error)

// Prepared function to revert the change after it has been applied
type RevertFunc func(interface{}) error

type ChangeManager struct {
	// Changes is a map of change names to Change objects, so that multiple changes can be tracked and reused.
	Changes map[string]*Change `yaml:"changes"`
	// Allowed must be set to true before any change can be applied.
	Allowed bool `yaml:"allowed,omitempty"`
	// BadState is true if any change has failed to apply or revert, indicating that the system may be in a bad state.
	BadState bool `yaml:"bad-state,omitempty"`
}

// Change is a struct that contains the data and functions associated with a single change to a target resource.
type Change struct {
	// TargetName is the name or ID of the resource or configuration that is to be changed
	TargetName string `yaml:"target-name"`
	// Description is a human-readable description of the change
	Description string `yaml:"description"`
	// applyFunc is the function that will be executed to make the change
	applyFunc ApplyFunc
	// revertFunc is the function that will be executed to undo the change
	revertFunc RevertFunc
	// TargetObject is a representation of the object that was changed
	TargetObject interface{} `yaml:"target-object,omitempty"`
	// Applied is true if the change was successfully applied at least once
	Applied bool `yaml:"applied,omitempty"`
	// Reverted is true if the change was successfully reverted and not applied again
	Reverted bool `yaml:"reverted,omitempty"`
	// Error is used if any error occurred during the change
	Error error `yaml:"error,omitempty"`
	// BadState is true if something went wrong during apply or revert, indicating that the system may be in a bad state
	BadState bool `yaml:"bad-state,omitempty"`
}

// Allow marks the change as allowed to be applied.
func (cm *ChangeManager) Allow() {
	cm.Allowed = true
}

func (cm *ChangeManager) Add(changeName string, change Change) {
	if cm.Changes == nil {
		cm.Changes = make(map[string]*Change)
	}
	cm.Changes[changeName] = &change
}

// Apply the prepared function for the change. It will not apply the change if it is not allowed, or if it has already been applied and not reverted.
func (cm *ChangeManager) Apply(changeName string, targetName string, changeInput any) (success bool, target any) {
	if !cm.Allowed {
		return false, nil
	}
	change, exists := cm.Changes[changeName]
	if !exists {
		return false, nil
	}
	success, target = change.apply(targetName, changeInput)
	if change.BadState {
		cm.BadState = true
	}
	return success, target
}

func (cm *ChangeManager) Revert(changeName string) {
	change, exists := cm.Changes[changeName]
	if !exists {
		return
	}
	change.revert(change.TargetObject)
	if change.BadState {
		cm.BadState = true
	}
}

// Apply the prepared function for the change. It will not apply the change if it has already been applied and not reverted.
// It will also not apply the change if it is not allowed.
func (c *Change) apply(targetName string, changeInput any) (success bool, target any) {
	err := c.precheck()
	// Return error if precheck fails
	if err != nil {
		c.Error = err
		return false, c.TargetObject
	}

	// Do nothing if the change has already been applied and not reverted
	if c.Applied && !c.Reverted {
		return true, c.TargetObject
	}

	c.TargetName = targetName
	c.TargetObject, err = c.applyFunc(changeInput)
	if err != nil {
		c.BadState = true
		c.Error = err
		return false, c.TargetObject
	}
	c.Applied = true
	c.Reverted = false
	return true, c.TargetObject
}

// Revert the change by executing the revert function. It does nothing if it has not been applied.
func (c *Change) revert(data interface{}) {
	if !c.Applied {
		return
	}
	err := c.precheck()
	if err != nil {
		c.Error = err
		return
	}
	err = c.revertFunc(data)
	if err != nil {
		c.Error = err
		c.BadState = true
		return
	}
	c.Reverted = true
}

// precheck verifies that the applyFunc and revertFunc are defined for the change.
// It returns an error if the change is not valid.
func (c *Change) precheck() error {
	if c.applyFunc == nil || c.revertFunc == nil {
		return fmt.Errorf("applyFunc and revertFunc must be defined for a change, but got applyFunc: %v, revertFunc: %v",
			c.applyFunc != nil, c.revertFunc != nil)
	}
	if c.TargetName == "" || c.Description == "" {
		return fmt.Errorf("change must have a TargetName and Description defined, but got TargetName: %v, Description: %v",
			c.TargetName, c.Description)
	}
	if c.Error != nil {
		return fmt.Errorf("change has a previous error and can no longer be applied: %s", c.Error.Error())
	}
	return nil
}

// NewChange creates a new Change object.
func NewChange(targetName string, description string, targetObject interface{}, applyFunc ApplyFunc, revertFunc RevertFunc) Change {
	return Change{
		TargetName:   targetName,
		TargetObject: targetObject,
		Description:  description,
		applyFunc:    applyFunc,
		revertFunc:   revertFunc,
	}
}
