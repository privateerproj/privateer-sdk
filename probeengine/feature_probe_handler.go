package probeengine

import (
	"bytes"
	"io"
	"log"
	"os"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/privateerproj/privateer-sdk/config"
)

// GodogProbeHandler is a wrapper to allow for multiple probe handlers in the future
func GodogProbeHandler(probe *GodogProbe) (int, *bytes.Buffer, error) {
	return toFileGodogProbeHandler(probe)
}

func toFileGodogProbeHandler(gd *GodogProbe) (int, *bytes.Buffer, error) {
	o, err := getOutputPath(gd.Name)
	if err != nil {
		return -1, nil, err
	}

	status := runTestSuite(o, gd)

	// If the tests are skipped due to tags, then an empty file may
	// be left lingering.  This will have a non-zero size as we've actually
	// had to create the file prior to the test run (see line 31).  If it's
	// less than 4 bytes, it's fairly certain that this will indeed be empty
	// and can be removed.
	i, err := o.Stat()
	s := i.Size()
	o.Close()
	if s < 4 {
		err = os.Remove(o.Name())
		if err != nil {
			log.Printf("[WARN] unable to remove empty test result file: %v", err)
		}
	}
	return status, nil, err
}

// Not currently in use, but leave this here for future reference
// This is how we might use probes within an application instead of CLI runtime
// func inMemGodogProbeHandler(gd *GodogProbe) (int, *bytes.Buffer, error) {
// 	var t []byte
// 	o := bytes.NewBuffer(t)
// 	status, err := runTestSuite(o, gd)
// 	return status, o, err
// }

func runTestSuite(o io.Writer, gd *GodogProbe) int {
	opts := godog.Options{
		Format: config.GlobalConfig.GodogResultsFormat,
		Output: colors.Colored(o),
		Paths:  []string{gd.FeaturePath},
		Tags:   gd.Tags,
	}

	status := godog.TestSuite{
		Name:                 gd.Name,
		TestSuiteInitializer: gd.ProbeInitializer,
		ScenarioInitializer:  gd.ScenarioInitializer,
		Options:              &opts,
	}.Run()

	return status
}
