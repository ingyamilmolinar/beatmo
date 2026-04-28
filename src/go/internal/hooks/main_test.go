package hooks

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enables goleak verification for hooks. We pre-warm the
// process-wide GlobalBus so its fan-out workers exist before
// IgnoreCurrent snapshots the goroutine set; anything spawned and
// leaked DURING a test still trips the check.
func TestMain(m *testing.M) {
	_ = GlobalBus() // force lazy init of hooks.fanout pool workers
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
