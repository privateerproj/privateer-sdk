package raidengine

import "testing"

var applyFunc = func() (interface{}, error) {
	return nil, nil
}
var revertFunc = func() error {
	return nil
}
var changes = []struct {
	testName string
	change   *Change
}{
	{
		testName: "Change not yet applied",
		change: &Change{
			applyFunc:  applyFunc,
			revertFunc: revertFunc,
			Applied:    false,
			Reverted:   false,
		},
	},
	{
		testName: "Change already applied and not yet reverted",
		change: &Change{
			applyFunc:  applyFunc,
			revertFunc: revertFunc,
			Applied:    true,
			Reverted:   false,
		},
	},
	{
		testName: "Change already applied and reverted",
		change: &Change{
			applyFunc:  applyFunc,
			revertFunc: revertFunc,
			Applied:    true,
			Reverted:   true,
		},
	},
	{
		testName: "No revert function specified (A)",
		change: &Change{
			applyFunc: applyFunc,
			Applied:   false,
			Reverted:  false,
		},
	},
	{
		testName: "No revert function specified (B)",
		change: &Change{
			applyFunc: applyFunc,
			Applied:   false,
			Reverted:  true,
		},
	},
	{
		testName: "No revert function specified (C)",
		change: &Change{
			applyFunc: applyFunc,
			Applied:   true,
			Reverted:  false,
		},
	},
	{
		testName: "No revert function specified (D)",
		change: &Change{
			applyFunc: applyFunc,
			Applied:   true,
			Reverted:  true,
		},
	},
	{
		testName: "No apply function specified (A)",
		change: &Change{
			revertFunc: revertFunc,
			Applied:    false,
			Reverted:   false,
		},
	},
	{
		testName: "No apply function specified (B)",
		change: &Change{
			revertFunc: revertFunc,
			Applied:    true,
			Reverted:   false,
		},
	},
	{
		testName: "No apply function specified (C)",
		change: &Change{
			revertFunc: revertFunc,
			Applied:    true,
			Reverted:   true,
		},
	},
	{
		testName: "Neither function specified (A)",
		change: &Change{
			Applied:  false,
			Reverted: false,
		},
	},
	{
		testName: "Neither function specified (B)",
		change: &Change{
			Applied:  true,
			Reverted: false,
		},
	},
	{
		testName: "Neither function specified (C)",
		change: &Change{
			Applied:  true,
			Reverted: true,
		},
	},
}

func TestApply(t *testing.T) {
	for _, c := range changes {
		t.Run(c.testName, func(t *testing.T) {

			c.change.Apply()

			if c.change.applyFunc == nil && c.change.Error == nil {
				t.Errorf("Expected error to be set due to nil applyFunc, but it was not")
			}
			if c.change.revertFunc == nil && c.change.Error == nil {
				t.Errorf("Expected error to be set due to nil revertFunc, but it was not")
			}
			if c.change.applyFunc != nil && c.change.revertFunc != nil {
				if !c.change.Applied {
					t.Errorf("Expected change to be applied, but it was not")
				}

				c.change.Revert()

				if !c.change.Applied {
					t.Errorf("Reverting shound not erase the record that the change was applied")
				}
			}
		})
	}
}

func TestRevert(t *testing.T) {
	for _, c := range changes {
		t.Run(c.testName, func(t *testing.T) {

			c.change.Revert()

			if c.change.applyFunc == nil && c.change.Error == nil {
				t.Errorf("Expected error to be set due to nil applyFunc, but it was not")
			}
			if c.change.revertFunc == nil && c.change.Error == nil {
				t.Errorf("Expected error to be set due to nil revertFunc, but it was not")
			}
			if c.change.applyFunc != nil && c.change.revertFunc != nil {
				if c.change.Applied && !c.change.Reverted {
					t.Errorf("Expected change to be reverted, but it was not")
				}
				if !c.change.Applied && c.change.Reverted {
					t.Errorf("Reverting should not be recorded if a change was not applied to revert")
				}
				c.change.Apply()

				if c.change.Reverted {
					t.Errorf("Applying further times shound mark the change as not reverted")
				}
			}

		})
	}
}
