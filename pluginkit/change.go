package pluginkit

import (
	"fmt"
	"log"
)

// Change is a struct that contains the data and functions associated with a single change
type Change struct {
	TargetName   string                      // TargetName is the name of the resource or configuration that was changed
	TargetObject interface{}                 // TargetObject is the object that was changed
	applyFunc    func() (interface{}, error) // Apply is the function that can be executed to make the change
	revertFunc   func() error                // Revert is the function that can be executed to revert the change
	Applied      bool                        // Applied is true if the change was successfully applied at least once
	Reverted     bool                        // Reverted is true if the change was successfully reverted and not applied again
	Error        error                       // Error is used if an error occurred during the change
}

// NewChange creates a new Change struct with the provided data
func NewChange(targetName string, targetObject interface{}, applyFunc func() (interface{}, error), revertFunc func() error) *Change {
	return &Change{
		TargetName:   targetName,
		TargetObject: targetObject,
		applyFunc:    applyFunc,
		revertFunc:   revertFunc,
	}
}

// Apply executes the Apply function for the change
func (c *Change) Apply() {
	err := c.precheck()
	if err != nil {
		c.Error = err
		return
	}
	// Do nothing if the change has already been applied and not reverted
	if c.Applied && !c.Reverted {
		return
	}
	obj, err := c.applyFunc()
	if err != nil {
		c.Error = err
		return
	}
	if obj != nil {
		c.TargetObject = obj
	}
	c.Applied = true
	c.Reverted = false
}

// Revert executes the Revert function for the change
func (c *Change) Revert() {
	err := c.precheck()
	if err != nil {
		c.Error = err
		return
	}
	// Do nothing if the change has not been applied
	if !c.Applied {
		return
	}
	err = c.revertFunc()
	if err != nil {
		c.Error = err
		return
	}
	c.Reverted = true
}

// precheck verifies that the applyFunc and revertFunc are defined for the change
func (c *Change) precheck() error {
	if c.applyFunc == nil {
		return fmt.Errorf("No apply function defined for change")
	}
	if c.revertFunc == nil {
		return fmt.Errorf("No revert function defined for change")
	}
	return nil
}

func revertTestChanges(tests *map[string]TestResult) (badStateAlert bool) {
	for testName, testResult := range *tests {
		for changeName, change := range testResult.Changes {
			if !badStateAlert && (change.Applied || change.Error != nil) {
				if !change.Reverted {
					change.Revert()
				}
				if change.Error != nil || !change.Reverted {
					badStateAlert = true
					log.Printf("[ERROR] Change in test '%s' failed to revert. Change name: %s", testName, changeName)
				}
			}
		}
	}
	return
}
