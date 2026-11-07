package async

import (
	"testing"

	"go.uber.org/goleak"
)

// Every Pool/Scheduler here is closed in a Cleanup, so a goroutine alive at exit is a leak.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
