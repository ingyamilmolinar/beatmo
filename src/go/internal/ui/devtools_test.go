package ui

import "testing"

// Build any Game after calling it: the tab strip and bottom nav are built from the available lists at construction.
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
