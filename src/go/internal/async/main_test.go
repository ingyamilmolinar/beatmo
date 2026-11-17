package async

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wraps the async package's tests with a goroutine-leak check.
// Every Pool/Scheduler in this package's tests is created with explicit
// Close in defer/t.Cleanup; if any worker goroutine is left running when
// the binary exits, that's a leak and must be fixed.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
