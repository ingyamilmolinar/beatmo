//go:build !test && !js

package audio

import (
	"os"
	"testing"
)

// Flushes gcov counters after the run: the Go runtime exits without C atexit handlers, so `make coverage-c` would
// find no .gcda data.
func TestMain(m *testing.M) {
	code := m.Run()
	FlushCCoverage()
	os.Exit(code)
}
