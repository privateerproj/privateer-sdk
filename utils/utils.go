// Package utils provides general utility methods.  The '*Ptr' functions were borrowed/inspired by the kubernetes go-client.
package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func init() {
}

// BoolPtr returns a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}

// StringPtr returns a pointer to the passed string.
func StringPtr(s string) *string {
	return &s
}

// Int64Ptr returns a pointer to an int64
func Int64Ptr(i int64) *int64 {
	return &i
}

// JSON marshals a struct into JSON with indentation
func JSON(data interface{}) []byte {
	j, _ := json.MarshalIndent(data, "", "  ")
	return []byte(j)
}

// FindString searches a []string for a specific value.
// If found, returns the index of first occurrence, and True. If not found, returns -1 and False.
func FindString(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

// CallerName retrieves the name of the function prior to the location it is called
// If using CallerName(0), the current function's name will be returned
// If using CallerName(1), the current function's parent name will be returned
// If using CallerName(2), the current function's parent's parent name will be returned
func CallerName(up int) string {
	s := strings.Split(CallerPath(up+1), ".") // split full caller path
	return s[len(s)-1]                        // select last element from caller path
}

// CallerPath checks the goroutine's stack of function invocation and returns the following:
// For up=0, return full caller path for caller function
// For up=1, returns full caller path for caller of caller
func CallerPath(up int) string {
	f := make([]uintptr, 1)
	runtime.Callers(up+2, f)                  // add full caller path to empty object
	return runtime.FuncForPC(f[0] - 1).Name() // get full caller path in string form
}

// CallerFileLine returns file name and line of invoker
// Similar to CallerName(1), but with file and line returned
func CallerFileLine() (string, int) {
	_, file, line, _ := runtime.Caller(2)
	return file, line
}

// ReformatError prefixes the error string ready for logging and/or output
func ReformatError(e string, v ...interface{}) error {
	var b strings.Builder
	b.WriteString("[ERROR] ")
	b.WriteString(e)

	s := fmt.Sprintf(b.String(), v...)

	return fmt.Errorf(s)
}

// ReplaceBytesValue replaces a substring with a new value for a given string in bytes
func ReplaceBytesValue(b []byte, old string, new string) []byte {
	newString := strings.Replace(string(b), old, new, -1)
	return []byte(newString)
}

// ReplaceBytesMultipleValues replaces multiple substring with a new value for a given string in bytes
func ReplaceBytesMultipleValues(b []byte, replacer *strings.Replacer) []byte {
	newString := replacer.Replace(string(b))
	return []byte(newString)
}

// WriteAllowed determines whether a given filepath can be written to
func WriteAllowed(path string) bool {
	_, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if os.IsPermission(err) {
		log.Printf("[ERROR] Permissions prevent this from writing to file: %s", path)
		return false
	} else if err != nil {
		log.Printf("[ERROR] Could not create or write to file: %s. Error: %s", path, err)
		return false
	}
	return true
}

// GetExecutableName returns name of executable without file extension
func GetExecutableName() string {
	execAbsPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Critical error ocurred while getting executable name")
	}

	execName := filepath.Base(execAbsPath)

	// Remove extension if it exists
	if ext := filepath.Ext(execName); ext != "" {
		execName = strings.TrimSuffix(execName, ext)
	}

	return execName
}
