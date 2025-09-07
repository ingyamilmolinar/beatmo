package ui

import (
    "flag"
    "os"
    "strings"
)

// runningUnderGoTest attempts to detect when the package is being executed
// as part of a "go test" binary so we can avoid side effects like building
// the interactive demo that pollute tests (both stubbed and real Ebiten).
func runningUnderGoTest() bool {
    // Heuristics: test binaries usually end with ".test" and include
    // specific flags injected by the testing package.
    if strings.HasSuffix(os.Args[0], ".test") {
        return true
    }
    if flag.Lookup("test.v") != nil || flag.Lookup("test.run") != nil || flag.Lookup("test.timeout") != nil {
        return true
    }
    return false
}

