//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"go.uber.org/goleak"
)

// TestMain enables goleak verification for audio tests. Pre-allocates
// the long-lived process-wide pools (recording.lifecycle, hooks.fanout)
// so their workers are part of the IgnoreCurrent baseline. Per-test
// recording sessions still need their own teardown (DrainWorkers +
// CloseWriters / WaitRecordingFinalized) or they'll trip the leak check.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("recording.lifecycle", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "recording.lifecycle",
	})
	_ = hooks.GlobalBus() // force lazy hooks.fanout pool allocation
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
