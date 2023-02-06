// Package probeengine contains the types and functions responsible for managing tests and test execution.  This is the primary
// entry point to the core of the application and should be utilised by the privateer library to create, execute and report
// on tests.
package probeengine

import (
	"errors"
	"log"
	"sync"

	audit "github.com/privateerproj/privateer-sdk/audit"
	"github.com/privateerproj/privateer-sdk/raidengine"

)

// ProbeStatus type describes the status of the test, e.g. Pending, Running, CompleteSuccess, CompleteFail and Error
type ProbeStatus int

//ProbeStatus enumeration for the ProbeStatus type.
const (
	Pending ProbeStatus = iota
	Running
	CompleteSuccess
	CompleteFail
	Error
	Excluded
)

func (s ProbeStatus) String() string {
	return [...]string{
		"Pending",
		"Running",
		"CompleteSuccess",
		"CompleteFail",
		"Error",
		"Excluded",
	}[s]
}

// ProbeStore maintains a collection of probes to be run and their status.  FailedProbes is an explicit
// collection of failed probes.
type ProbeStore struct {
	Name         string
	Probes       map[string]*GodogProbe
	FailedProbes map[ProbeStatus]*GodogProbe
	Lock         sync.RWMutex
	Summary      *audit.SummaryState
	Tags         string
}

// NewProbeStore creates a new object to store GodogProbes
func NewProbeStore(name string, tags string, summaryState *audit.SummaryState) *ProbeStore {
	log.Printf("[INFO] Creating new Probe store with tags: %s", tags)
	return &ProbeStore{
		Name:    name,
		Probes:  make(map[string]*GodogProbe),
		Summary: summaryState,
		Tags:    tags,
	}
}

// RunAllProbes retrieves and executes all probes that have been included
func (ps *ProbeStore) RunAllProbes(probes []raidengine.Strike) (exitCode int, err error) {
	for _, probe := range probes {
		err = probe()
		if err != nil {
			return 1, err
		}
	}
	return 0, nil
}

// AddProbe provided GodogProbe to the ProbeStore.
func (ps *ProbeStore) AddProbe(preParsedProbe Probe) {
	ps.Lock.Lock()
	defer ps.Lock.Unlock()

	probe := ps.makeGodogProbe(ps.Name, preParsedProbe)
	status := Pending
	probe.Status = &status
	ps.Probes[probe.Name] = probe

	ps.Summary.GetProbeLog(probe.Name).Result = probe.Status.String()
	ps.Summary.LogProbeMeta(probe.Name, "group", probe.Pack)
}

// GetProbe returns the test identified by the given name.
func (ps *ProbeStore) GetProbe(name string) (*GodogProbe, error) {
	ps.Lock.Lock()
	defer ps.Lock.Unlock()

	//get the test from the store
	p, exists := ps.Probes[name]

	if !exists {
		return nil, errors.New("test with name '" + name + "' not found")
	}
	return p, nil
}

// ExecProbe executes the test identified by the specified name.
func (ps *ProbeStore) ExecProbe(name string) (int, error) {
	p, err := ps.GetProbe(name)
	if err != nil {
		return 1, err // Failure
	}
	if p.Status.String() != Excluded.String() {
		return ps.RunProbe(p) // Return test results
	}
	return 0, nil // Succeed if test is excluded
}

// ExecAllProbes executes all tests that are present in the ProbeStore.
func (ps *ProbeStore) ExecAllProbes() (int, error) {
	status := 0
	var err error

	for name := range ps.Probes {
		st, err := ps.ExecProbe(name)
		ps.Summary.ProbeComplete(name)
		if err != nil {
			//log but continue with remaining probe
			log.Printf("[ERROR] error executing probe: %v", err)
		}
		if st > status {
			status = st
		}
	}
	ps.Summary.SetPrivateerStatus()
	return status, err
}

func (ps *ProbeStore) makeGodogProbe(pack string, probe Probe) *GodogProbe {
	return &GodogProbe{
		Name:                probe.Name(),
		Pack:                pack,
		ProbeInitializer:    probe.ProbeInitialize,
		ScenarioInitializer: probe.ScenarioInitialize,
		FeaturePath:         probe.Path(),
		Tags:                ps.Tags,
	}
}
