package raidengine

import (
	"log"
	"reflect"
	"runtime"
)

type Strike func() error

func Run(strikes []Strike) (errors []error) {
	for _, strike := range strikes {
		err := execStrike(strike)
		if err != nil {
			errors = append(errors, err)
		}
	}
	return
}

func execStrike(strike Strike) error {
	log.Printf("Initiating Strike: %v", GetFunctionAddress(strike))
	err := strike()
	if err != nil {
		log.Print(err)
	}
	return err
}

func GetFunctionAddress(i Strike) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
