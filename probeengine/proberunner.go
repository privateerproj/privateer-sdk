package probeengine

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/cucumber/godog"
	"github.com/privateerproj/privateer-sdk/config"
)

// ProbeRunner describes the interface that should be implemented to support the execution of tests.
type ProbeRunner interface {
	RunProbe(t *GodogProbe) error
}

// ProbeHandlerFunc describes a callback that should be implemented by test cases in order for ProbeRunner
// to be able to execute the test case.
type ProbeHandlerFunc func(t *GodogProbe) (int, *bytes.Buffer, error)

// GodogProbe encapsulates the specific data that GoDog feature based tests require in order to run.   This
// structure will be passed to the test handler callback.
type GodogProbe struct {
	Name                string
	Pack                string
	ProbeInitializer    func(*godog.TestSuiteContext)
	ScenarioInitializer func(*godog.ScenarioContext)
	FeaturePath         string
	Status              *ProbeStatus
	Results             *bytes.Buffer
	Tags                string
}

// RunProbe runs the test cases described by the supplied Probe
func (ps *ProbeStore) RunProbe(probe *GodogProbe) (int, error) {

	if probe == nil {
		ps.Summary.GetProbeLog(probe.Name).Result = "Internal Error - Probe not found"
		return 2, fmt.Errorf("probe is nil - cannot run test")
	}

	s, o, err := GodogProbeHandler(probe)

	if s == 0 {
		// success
		*probe.Status = CompleteSuccess
	} else {
		// fail
		*probe.Status = CompleteFail
	}

	probe.Results = o // If in-mem output provided, store as Results
	return s, err
}

// GetAllProbeResults maps ProbeStore results to strings
// Designed for use with in-memory output, such as for an API runtime
func GetAllProbeResults(ps *ProbeStore) (allResults map[string]string) {
	allResults = make(map[string]string)
	for name := range ps.Probes {
		probeResults, name, err := readProbeResults(ps, name)
		if err != nil {
			allResults[name] = err.Error()
		} else {
			allResults[name] = probeResults
		}
	}
	return
}

func readProbeResults(ps *ProbeStore, name string) (probeResults, probeName string, err error) {
	p, err := ps.GetProbe(name)
	if err != nil {
		return
	}
	probeResults = p.Results.String()
	probeName = p.Name
	return
}

// CleanupTmp is used to dispose of any temp resources used during execution
func CleanupTmp() {
	err := os.RemoveAll(config.GlobalConfig.TmpDir)
	if err != nil {
		log.Printf("[ERROR] Error removing tmp folder %v", err)
	}
}
