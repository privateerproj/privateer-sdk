package raidengine

import (
	"fmt"
	"log"
	"reflect"
	"runtime"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
)

type Strike func() error

// Run is used to execute a list of strikes, intended to be pre-parsed by UniqueAttacks
func Run(strikes []Strike) (errors []error) {
	for _, strike := range strikes {
		err := execStrike(strike)
		if err != nil {
			errors = append(errors, err)
		}
	}
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
