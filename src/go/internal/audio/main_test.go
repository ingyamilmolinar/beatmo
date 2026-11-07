//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"go.uber.org/goleak"
)

// The long-lived pools are created first so their workers are in goleak's IgnoreCurrent baseline. Per-test recording
// sessions still need their own teardown.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("recording.lifecycle", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "recording.lifecycle",
	})
	_ = hooks.GlobalBus() // force lazy hooks.fanout pool allocation
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
