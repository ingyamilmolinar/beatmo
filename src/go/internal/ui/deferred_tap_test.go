//go:build test

package ui

import "testing"

func TestDeferredTapBeginEnd(t *testing.T) {
	assertDefaultParityState(t)

	var dt DeferredTap

	// Begin captures position.
	if !dt.Begin(10, 20) {
		t.Fatal("Begin should succeed when suppress is off")
	}
	if !dt.Active() {
		t.Fatal("should be active after Begin")
	}
	x, y := dt.Pos()
	if x != 10 || y != 20 {
		t.Fatalf("Pos = (%d,%d), want (10,20)", x, y)
	}

	// End fires callback with stored position.
	var fx, fy int
	fired := dt.End(func(x, y int) { fx, fy = x, y })
	if !fired {
		t.Fatal("End should return true when active")
	}
	if fx != 10 || fy != 20 {
		t.Fatalf("fire callback got (%d,%d), want (10,20)", fx, fy)
	}
	if dt.Active() {
		t.Fatal("should not be active after End")
	}
}

func TestDeferredTapEndWithoutBegin(t *testing.T) {
	assertDefaultParityState(t)

	var dt DeferredTap
	fired := dt.End(func(x, y int) {
		t.Fatal("fire callback should not be called when not active")
	})
	if fired {
		t.Fatal("End should return false when not active")
	}
}

func TestDeferredTapSuppressGuard(t *testing.T) {
	assertDefaultParityState(t)

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var dt DeferredTap
	if dt.Begin(5, 5) {
		t.Fatal("Begin should return false when suppress is active")
	}
	if dt.Active() {
		t.Fatal("should not be active when Begin was suppressed")
	}
}

func TestDeferredTapCancel(t *testing.T) {
	assertDefaultParityState(t)

	var dt DeferredTap
	dt.Begin(1, 2)
	if !dt.Active() {
		t.Fatal("should be active after Begin")
	}

	dt.Cancel()
	if dt.Active() {
		t.Fatal("should not be active after Cancel")
	}

	// End after Cancel should be a no-op.
	fired := dt.End(func(x, y int) {
		t.Fatal("fire callback should not be called after Cancel")
	})
	if fired {
		t.Fatal("End should return false after Cancel")
	}
}

func TestDeferredTapEndNilCallback(t *testing.T) {
	assertDefaultParityState(t)

	var dt DeferredTap
	dt.Begin(3, 4)
	// End with nil callback should still clear state without panicking.
	fired := dt.End(nil)
	if !fired {
		t.Fatal("End should return true when active, even with nil callback")
	}
	if dt.Active() {
		t.Fatal("should not be active after End")
	}
}
