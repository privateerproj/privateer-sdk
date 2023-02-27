package raidengine

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
)

type Strike func() error
type cleanupFunc func() error

var cleanup = func() error {
	log.Printf("[ERROR] No custom cleanup specified by this raid") // Default to be overriden by SetupCloseHandler
	return nil
}

// Run is used to execute a list of strikes, intended to be pre-parsed by UniqueAttacks
func Run(strikes []Strike) (errors []error) {
	closeHandler()
	for _, strike := range strikes {
		err := execStrike(strike)
		if err != nil {
			errors = append(errors, err)
		}
	}
	cleanup()
	writeRaidLog(errors)
	return
}

func execStrike(strike Strike) error {
	log.Printf("Initiating Strike: %v", getFunctionAddress(strike))
	err := strike()
	if err != nil {
		log.Print(err)
	}
	return err
}

func writeRaidLog(errors []error) {
	config.GlobalConfig.PrepareOutputDirectory()
	// TODO: Get user feedback on desired output
	// for i, err := range errors {
	// 	log.Printf("%v: %v", i, err)
	// }
}

func GetUniqueStrikes(strikePacks map[string][]Strike, policies ...string) (strikes []Strike) {
	log.Printf("Policies Requested: %s", strings.Join(policies, ","))

	if len(policies) == 1 {
		// If set via environment variables, this value may come in as a comma delineated string
		policies = strings.Split(policies[0], ",")
	}
	for _, strike := range policies {
		if _, ok := strikePacks[strike]; !ok {
			log.Print(utils.ReformatError("Strike pack not found for policy: %s (Skipping)", strike))
			continue
		}
		strikes = append(strikes, strikePacks[strike]...)
	}
	return uniqueStrikes(strikes)
}

func uniqueStrikes(allStrikes []Strike) (strikes []Strike) {
	used := make(map[string]bool)
	for _, strike := range allStrikes {
		name := getFunctionAddress(strike)
		if _, ok := used[name]; !ok {
			used[name] = true
			strikes = append(strikes, strike)
		}
	}
	return
}

func getFunctionAddress(i Strike) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func GetOutput(raids []Strike, errors []error) error {
	raidsRequested := len(raids)
	raidsSucceeded := len(raids) - len(errors)
	output := fmt.Sprintf("%v/%v attacks succeeded. View the output logs for more details.", raidsSucceeded, raidsRequested)

	if errors != nil {
		return utils.ReformatError(output)
	}

	log.Print(output)
	return nil
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func SetupCloseHandler(customFunction cleanupFunc) {
	cleanup = customFunction
}

func closeHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("Execution aborted - %v", "SIGTERM")
		defer config.GlobalConfig.CleanupTmp()
		if err := cleanup(); err != nil {
			log.Printf("[ERROR] Cleanup may not be complete. %v", err.Error()) // Perform any custom cleanup for the
		}
		os.Exit(0)
	}()
}
