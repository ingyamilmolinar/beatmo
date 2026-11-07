package hooks

import (
	"testing"

	"go.uber.org/goleak"
)

// GlobalBus's workers exist before IgnoreCurrent snapshots the baseline; anything leaked during a test still trips it.
func TestMain(m *testing.M) {
	_ = GlobalBus() // force lazy init of hooks.fanout pool workers
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
