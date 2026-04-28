package eventlogger

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"go.uber.org/goleak"
)

// TestMain enables goleak verification. Pre-allocate the standing pool so
// its worker goroutine exists before IgnoreCurrent snapshots the baseline;
// tests that legitimately leak (Loggers not Closed) will still trip.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("eventlogger.format", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "eventlogger.format",
	})
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
