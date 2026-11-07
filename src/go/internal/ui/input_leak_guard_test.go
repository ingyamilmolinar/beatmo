package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// Never mix `defer restore()` with t.Cleanup-based setTestInput: defers run first, so the cleanups walk the
// globals back up the stack and leave a mock live for the rest of the process. A mock returning exactly what
// Ebiten returns is undetectable here, an accepted limit.
func inputGlobalsLeakMsg() string {
	if touchOverrideActive {
		// The touch override legitimately diverges cursorPosition() from the raw binding; resetTouchOverride clears it.
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
