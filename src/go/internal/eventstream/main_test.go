package eventstream

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"go.uber.org/goleak"
)

// The shared persist pool's worker exists before IgnoreCurrent snapshots the baseline; unclosed sinks still trip it.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("eventstream.persist", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "eventstream.persist",
	})
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
