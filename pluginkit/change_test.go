package pluginkit

import (
	"errors"
)

var (
	// Functions
	goodApplyFunc = func(interface{}) (interface{}, error) {
		return nil, nil
	}
	goodRevertFunc = func(interface{}) error {
		return nil
	}
	badApplyFunc = func(interface{}) (interface{}, error) {
		return nil, errors.New("error")
	}
	badRevertFunc = func(interface{}) error {
		return errors.New("error")
	}
)

func changesTestData() []struct {
	testName string
	change   Change
} {
	return []struct {
		testName string
		change   Change
	}{
		{
			testName: "Change not yet applied",
			change:   pendingChange(),
		},
		{
			testName: "Change already applied and not yet reverted",
			change:   appliedNotRevertedChange(),
		},
		{
			testName: "Change already applied and reverted",
			change:   appliedRevertedChange(),
		},
		{
			testName: "No revert function specified",
			change:   noRevertChange(),
		},
		{
			testName: "No apply function specified",
			change:   noApplyChange(),
		},
		{
			testName: "Neither function specified",
			change:   Change{},
		},
		{
			testName: "Change is not allowed to execute",
			change:   disallowedChange(),
		},
	}
}

func pendingChangePtr() *Change {
	c := pendingChange()
	return &c
}
func pendingChange() Change {
	return Change{
		TargetName:  "pendingChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
	}
}
func appliedRevertedChange() Change {
	return Change{
		TargetName:  "appliedRevertedChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
		Applied:     true,
		Reverted:    true,
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
func badApplyChangePtr() *Change {
	c := badApplyChange()
	return &c
}
func badApplyChange() Change {
	return Change{
		TargetName:  "badApplyChange",
		Description: "description placeholder",
		applyFunc:   badApplyFunc,
		revertFunc:  goodRevertFunc,
	}
}
func badRevertChangePtr() *Change {
	c := badRevertChange()
	return &c
}
func badRevertChange() Change {
	return Change{
		TargetName:  "badRevertChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  badRevertFunc,
	}
}
func goodRevertedChangePtr() *Change {
	c := goodRevertedChange()
	return &c
}
func goodRevertedChange() Change {
	return Change{
		TargetName:  "goodRevertedChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
		Reverted:    true,
	}
}
func goodNotRevertedChangePtr() *Change {
	c := goodNotRevertedChange()
	return &c
}
func goodNotRevertedChange() Change {
	return Change{
		TargetName:  "goodNotRevertedChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
		Applied:     true,
	}
}
func noApplyChangePtr() *Change {
	c := noApplyChange()
	return &c
}
func noApplyChange() Change {
	return Change{
		TargetName:  "noApplyChange",
		Description: "description placeholder",
		revertFunc:  goodRevertFunc,
	}
}
func noRevertChange() Change {
	return Change{
		TargetName:  "noRevertChange",
		Description: "description placeholder",
		applyFunc:   goodApplyFunc,
	}
}
func disallowedChange() Change {
	return Change{
		TargetName:  "disallowedChange",
		Description: "description placeholder",
		Allowed:     false,
		applyFunc:   goodApplyFunc,
		revertFunc:  goodRevertFunc,
	}
}
