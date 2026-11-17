package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// inputGlobalsLeakMsg reports whether a test left an input mock installed in
// the package-level input function variables (input.go). It returns "" when
// the globals are pristine, else a diagnostic to print before failing the run.
//
// # WHY THIS GUARD EXISTS
//
// cursorPosition / isMouseButtonPressed / ... are package-level function
// VARIABLES that every input test swaps out and is expected to restore. A
// test that leaks one silently re-aims every LATER test's cursor, and the
// damage lands nowhere near the culprit: the leaked mock reported (512,10),
// which sat outside the node sidebar's panel, so the dispatcher stopped
// calling NodeSidebar.HandleInput -> layout() and three sidebar tests failed
// on stale rects — but ONLY under the real Ebiten build, and ONLY in a full
// suite run. In isolation, and under the stubbed build (whose mock cursor
// defaults to (0,0), inside the panel), they passed.
//
// The leak survived every "restore" being called, because balance is not
// enough — ORDER matters. The offending shape was:
//
//	restore := SetInputForTest(...)  // A: saves the real funcs
//	defer restore()                  // runs FIRST, restores the real funcs
//	setTestInput(t, ...)             // B: saves A, restores via t.Cleanup
//	setTestInput(t, ...)             // C: saves B, restores via t.Cleanup
//
// Go runs deferred functions BEFORE t.Cleanup functions, so the cleanups ran
// last and walked the globals back UP the stack: C reinstalled B's mock, then
// B reinstalled A's mock — leaving A's mock live for the rest of the process.
// Every restore ran exactly once; the process still ended up mocked.
//
// The rule this guard enforces: never mix `defer restore()` with the
// t.Cleanup-based helpers (setTestInput). Pick one, so restores unwind in
// LIFO order and the last one puts the real functions back.
//
// Detection compares the exported variable against the raw Ebiten binding it
// wraps. A leaked mock that happens to return exactly what real Ebiten
// returns is not detectable this way and will slip through — that is an
// accepted limit, since the mocks that actually cause damage are the ones
// reporting a different cursor.
func inputGlobalsLeakMsg() string {
	if touchOverrideActive {
		// The touch override legitimately makes cursorPosition() diverge from
		// the raw binding; a leaked override is goleak-adjacent but is reset
		// by resetTouchOverride and is not what this guard is about.
		return "input globals leak: touchOverrideActive is still set after the suite"
	}
	wx, wy := cursorPosition()
	rx, ry := ebiten.CursorPosition()
	if wx != rx || wy != ry {
		return fmt.Sprintf(
			"input globals leak: cursorPosition() = (%d,%d) but real ebiten.CursorPosition() = (%d,%d)\n"+
				"A test replaced cursorPosition and never put it back, so every later test in this\n"+
				"package saw a phantom cursor. Most likely cause: a test mixed `defer restore()`\n"+
				"with setTestInput(t, ...) — defers run BEFORE t.Cleanup, so the cleanups reinstall\n"+
				"the earlier mock. Use setTestInput(t, ...) for every install in a test, or defer for\n"+
				"every install, but never both. See inputGlobalsLeakMsg in input_leak_guard_test.go.",
			wx, wy, rx, ry)
	}
	if inputForTestActive {
		return "input globals leak: inputForTestActive is still true after the suite — a SetInputForTest restore never ran"
	}
	return ""
}
