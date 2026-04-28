package engine

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enables goleak verification for the engine package. The
// Predictor's background worker is per-instance and stopped via
// StopBackground in tests; anything that escapes will be flagged.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
