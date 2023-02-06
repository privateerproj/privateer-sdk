package audit

import (
	"github.com/cucumber/messages/go/v21"
)

// Probe is passed through various functions to audit the probe's progress
type Probe struct {
	name               string
	Meta               map[string]interface{}
	Path               string
	ScenariosAttempted int
	ScenariosSucceeded int
	ScenariosFailed    int
	GivenNotMet        int
	Result             string
	Scenarios          map[int]*Scenario
}

type limitedProbe struct {
	Meta               map[string]interface{} `json:"Meta"`
	Path               string                 `json:"Path"`
	ScenariosAttempted int                    `json:"ScenariosAttempted"`
	ScenariosSucceeded int                    `json:"ScenariosSucceeded"`
	ScenariosFailed    int                    `json:"ScenariosFailed"`
	GivenNotMet        int                    `json:"GivenNotMet"`
	Result             string                 `json:"Result"`
}

// countResults stores the current total number of failures as e.ScenariosFailed. Run at probe end
func (e *Probe) countResults() {
	e.ScenariosAttempted = len(e.Scenarios)
	for _, v := range e.Scenarios {
		if v.Result == "Failed" {
			e.ScenariosFailed = e.ScenariosFailed + 1
		} else if v.Result == "Passed" {
			e.ScenariosSucceeded = e.ScenariosSucceeded + 1
		} else if v.Result == "Given Not Met" {
			e.GivenNotMet = e.GivenNotMet + 1
		}
	}
}

// InitializeAuditor creates a new audit entry for the specified scenario
func (e *Probe) InitializeAuditor(name string, tags []*messages.PickleTag) *Scenario {
	if e.Scenarios == nil {
		e.Scenarios = make(map[int]*Scenario)
	}
	i := len(e.Scenarios) + 1
	var t []string
	for _, tag := range tags {
		t = append(t, tag.Name)
	}
	e.Scenarios[i] = &Scenario{
		Name:  name,
		Steps: make(map[int]*step),
		Tags:  t,
	}
	return e.Scenarios[i]
}
