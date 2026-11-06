package ui

import "testing"

// forceProductionUIForTest makes devToolsOn() report false for the rest of the
// test (the production UI beatmo.io ships), restoring the previous state on
// cleanup. Build any Game AFTER calling it: the tab strip and the bottom nav
// are built from the available-tab lists at construction.
func forceProductionUIForTest(t testing.TB) {
	t.Helper()
	prev := devToolsForcedOff.Load()
	SetDevToolsForcedOff(true)
	t.Cleanup(func() { SetDevToolsForcedOff(prev) })
}

func TestDevToolsPredicate(t *testing.T) {
	if !devToolsBuild {
		t.Skip("release build: developer tools are off by construction")
	}
	if !devToolsOn() {
		t.Fatal("a developer build must start with developer tools on")
	}
	forceProductionUIForTest(t)
	if devToolsOn() {
		t.Fatal("SetDevToolsForcedOff(true) must turn developer tools off")
	}
	SetDevToolsForcedOff(false)
	if !devToolsOn() {
		t.Fatal("SetDevToolsForcedOff(false) must turn developer tools back on in a developer build")
	}
}
