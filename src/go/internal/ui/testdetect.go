package ui

import (
	"flag"
	"os"
	"strings"
)

// So test binaries skip side effects like building the interactive demo.
func runningUnderGoTest() bool {
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	if flag.Lookup("test.v") != nil || flag.Lookup("test.run") != nil || flag.Lookup("test.timeout") != nil {
		return true
	}
	return false
}
