//go:build !js

package userprefs

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enables goleak verification. Per-test pools are created with
// unique names and released by t.Cleanup, so the IgnoreCurrent baseline
// only includes test-runner goroutines + whatever async/hooks pool the
// registry has already lazily started.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
