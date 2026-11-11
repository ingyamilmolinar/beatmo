package eventstream

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"go.uber.org/goleak"
)

// TestMain enables goleak verification for eventstream. We pre-allocate
// the shared eventstream.persist pool directly via the registry so its
// worker goroutine exists before IgnoreCurrent snapshots the goroutine
// set; tests that legitimately leak goroutines (sinks not closed,
// subscriptions not unsubscribed) will still trip the check.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("eventstream.persist", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "eventstream.persist",
	})
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
