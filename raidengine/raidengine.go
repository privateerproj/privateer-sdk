package raidengine

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"

	"github.com/hashicorp/go-hclog"

	"github.com/privateerproj/privateer-sdk/command"
	"github.com/privateerproj/privateer-sdk/logging"
	"github.com/privateerproj/privateer-sdk/utils"
	"github.com/spf13/viper"
)

type Strike func() error
type cleanupFunc func() error

var logger hclog.Logger

var cleanup = func() error {
	logger.Debug("No custom cleanup specified by this raid") // Default to be overriden by SetupCloseHandler
	return nil
}

// Run is used to execute a list of strikes, intended to be pre-parsed by UniqueAttacks
func Run(name string, availableStrikes map[string][]Strike) error {
	logger = logging.GetLogger("cli", viper.GetString("loglevel"), true)
	closeHandler()
	var errs []error
	strikes := availableStrikes["CIS"]

	for _, strike := range strikes {
		err := execStrike(strike)
		if err != nil {
			errs = append(errs, err)
		}
	}

	cleanup()
	writeRaidLog(errs)
	output := fmt.Sprintf(
		"%s: %v/%v attacks succeeded. View the output logs for more details.", name, len(strikes)-len(errs), len(strikes))
	logger.Info(output)
	if len(errs) > 0 {
		return errors.New(output)
	}
	return nil
}

func execStrike(strike Strike) error {
	logger.Debug("Initiating Strike: %v", getFunctionAddress(strike))
	err := strike()
	if err != nil {
		log.Print(err)
	}
	return err
}

func writeRaidLog(errors []error) {
	// TODO: Get user feedback on desired output
	// for i, err := range errors {
	// 	log.Printf("%v: %v", i, err)
	// }
}

func GetUniqueStrikes(strikePacks map[string][]Strike, policies ...string) (strikes []Strike) {
	logger.Debug(fmt.Sprintf(
		"Policies Requested: %s", strings.Join(policies, ",")))

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

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func SetupCloseHandler(customFunction cleanupFunc) {
	cleanup = customFunction
}

func closeHandler() {
	command.InitializeConfig()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Error("Execution aborted - %v", "SIGTERM")
		// defer cleanupTmp() TODO: replace the old logic that was here
		if cleanup != nil {
			if err := cleanup(); err != nil {
				logger.Error("Cleanup returned an error, and may not be complete: %v", err.Error())
			}
		} else {
			logger.Trace("No custom cleanup was provided by the terminated Raid.")
		}
		os.Exit(0)
	}()
}
