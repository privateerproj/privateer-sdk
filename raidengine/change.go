package raidengine

import "fmt"

// Change is a struct that contains the data and functions associated with a single change
type Change struct {
	TargetName   string       // TargetName is the name of the resource or configuration that was changed
	TargetObject interface{}  // TargetObject is the object that was changed
	ApplyFunc    func() error // Apply is the function that can be executed to make the change
	RevertFunc   func() error // Revert is the function that can be executed to revert the change
	Applied      bool         // Applied is true if the change was successfully applied at least once
	Reverted     bool         // Reverted is true if the change was successfully reverted and not applied again
	Error        error        // Error is used if an error occurred during the change
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
	err = c.ApplyFunc()
	if err != nil {
		c.Error = err
		return
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
	err = c.RevertFunc()
	if err != nil {
		c.Error = err
		return
	}
	c.Reverted = true
}

// precheck verifies that the ApplyFunc and RevertFunc are defined for the change
func (c *Change) precheck() error {
	if c.ApplyFunc == nil {
		return fmt.Errorf("No apply function defined for change")
	}
	if c.RevertFunc == nil {
		return fmt.Errorf("No revert function defined for change")
	}
	return nil
}
