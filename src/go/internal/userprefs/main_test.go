//go:build !js

package userprefs

import (
	"testing"

	"go.uber.org/goleak"
)

// Per-test pools are uniquely named and released, so the baseline holds only runner goroutines and pools the registry
// already started.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
