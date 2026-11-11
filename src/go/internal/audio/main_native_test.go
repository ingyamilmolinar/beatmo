//go:build !test && !js

package audio

import (
	"os"
	"testing"
)

// TestMain (native build) mirrors the test-tag main_test.go entry point but
// additionally flushes gcov counters after the run: scripts/coverage-c.sh
// links the C DSP library with --coverage, and the Go runtime exits without
// running C atexit handlers, so without this explicit dump the .gcda files
// would never be written and `make coverage-c` would report no data.
func TestMain(m *testing.M) {
	code := m.Run()
	FlushCCoverage()
	os.Exit(code)
}
