package audit

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/privateerproj/privateer-sdk/utils"
)

// Scenario is used by scenario states to audit progress through each step
type Scenario struct {
	Name   string
	Result string // Passed / Failed / Given Not Met
	Tags   []string
	Steps  map[int]*step
}

type step struct {
	Function    string
	Name        string
	Description string      // Long-form explanation of anything happening in the step
	Result      string      // Passed / Failed
	Error       string      // Log the error text
	Payload     interface{} // Handles any values that are sent across the network
}

func (e *Probe) Write() {
	if len(e.Scenarios) > 0 && utils.WriteAllowed(e.Path) {
		os.Create(e.Path)
		json, _ := json.MarshalIndent(e, "", "  ")
		data := []byte(json)
		ioutil.WriteFile(e.Path, data, 0755)
	}
}

// AuditScenarioStep sets description, payload, and pass/fail based on err parameter.
// This function should be deferred to catch panic behavior, otherwise the audit will not be logged on panic
func (p *Scenario) AuditScenarioStep(stepName, description string, payload interface{}, err error) {
	stepFunctionName := utils.CallerName(2) // returns name if deferred and not panicking
	switch stepFunctionName {
	case "call":
		stepFunctionName = utils.CallerName(1) // returns name if this function was not deferred in the caller
	case "gopanic":
		stepFunctionName = utils.CallerName(3) // returns name if caller panicked and this function was deferred
	}

	p.audit(stepFunctionName, stepName, description, payload, err)
}

func (p *Scenario) audit(functionName string, stepName string, description string, payload interface{}, err error) {
	stepNumber := len(p.Steps) + 1
	p.Steps[stepNumber] = &step{
		Function:    functionName,
		Name:        stepName,
		Description: description,
		Payload:     payload,
	}
	if err == nil {
		p.Steps[stepNumber].Result = "Passed"
		p.Result = "Passed"
	} else {
		p.Steps[stepNumber].Result = "Failed"
		p.Steps[stepNumber].Error = strings.Replace(err.Error(), "[ERROR] ", "", -1)
		if stepNumber == 1 {
			// TODO: change to handle this in AuditScenarioGiven, then here do if step.IsGiven
			p.Result = "Given Not Met" // First entry is always a 'given'; failures should be ignored
		} else {
			p.Result = "Failed" // First 'given' was met, but a subsequent step failed
		}
	}
}
