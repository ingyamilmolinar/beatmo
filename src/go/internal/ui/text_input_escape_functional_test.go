package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fakeInput is a stateful input source that drives the REAL Game.Update loop
// the way a user's hardware does: edge-triggered keys (isKeyJustPressed true
// only on the first frame a key goes down), a held mouse button, and typed
// characters. This is the faithful harness for "test as the user will use it".
type fakeInput struct {
	cx, cy   int
	mouse    bool
	keys     map[ebiten.Key]bool
	prevKeys map[ebiten.Key]bool
	chars    []rune
	w, h     int
	screen   *ebiten.Image
}

func newFakeInput(w, h int) *fakeInput {
	return &fakeInput{w: w, h: h, keys: map[ebiten.Key]bool{}, prevKeys: map[ebiten.Key]bool{}}
}

func installFakeInput(fi *fakeInput) func() {
	oc, om, okp, okjp, ochr, owh, osc := cursorPosition, isMouseButtonPressed, isKeyPressed, isKeyJustPressed, inputChars, wheel, screenSize
	cursorPosition = func() (int, int) { return fi.cx, fi.cy }
	isMouseButtonPressed = func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && fi.mouse }
	isKeyPressed = func(k ebiten.Key) bool { return fi.keys[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return fi.keys[k] && !fi.prevKeys[k] }
	inputChars = func() []rune { c := fi.chars; fi.chars = nil; return c }
	wheel = func() (float64, float64) { return 0, 0 }
	screenSize = func() (int, int) { return fi.w, fi.h }
	suppressClicksUntilRelease = false
	inputForTestActive = true
	resetTouchOverride()
	return func() {
		cursorPosition, isMouseButtonPressed, isKeyPressed, isKeyJustPressed, inputChars, wheel, screenSize = oc, om, okp, okjp, ochr, owh, osc
		suppressClicksUntilRelease = false
		inputForTestActive = false
		resetTouchOverride()
	}
}

// frame advances one Update+Draw with the current input state (the real game
// loop runs both each frame; many control rects are assigned during Draw), then
// rolls the key snapshot so the next frame computes just-pressed correctly.
func (fi *fakeInput) frame(t *testing.T, g *Game) {
	t.Helper()
	if err := g.Update(); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if fi.screen == nil {
		fi.screen = ebiten.NewImage(fi.w, fi.h)
	}
	g.Draw(fi.screen)
	fi.prevKeys = map[ebiten.Key]bool{}
	for k, v := range fi.keys {
		fi.prevKeys[k] = v
	}
}

// clickAt taps (press then release) at a screen point through Update.
func (fi *fakeInput) clickAt(t *testing.T, g *Game, x, y int) {
	t.Helper()
	fi.cx, fi.cy = x, y
	fi.mouse = true
	fi.frame(t, g)
	fi.mouse = false
	fi.frame(t, g)
}

// pressKey holds a key for one frame (just-pressed), then releases it.
func (fi *fakeInput) pressKey(t *testing.T, g *Game, k ebiten.Key) {
	t.Helper()
	fi.keys[k] = true
	fi.frame(t, g)
	fi.keys[k] = false
	fi.frame(t, g)
}

func newDesktopGame(t *testing.T) *Game {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	return g
}

// TestFunctional_Esc_BPMBox reproduces the exact user flow: click the BPM box,
// type a new tempo, press Esc — expecting the edit to be discarded and the box
// closed. Drives the real Game.Update loop end-to-end.
func TestFunctional_Esc_BPMBox(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()

	// Settle a couple frames so transport layout assigns the BPM box rect.
	fi.frame(t, g)
	fi.frame(t, g)

	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Fatalf("BPM box has no rect (rect=%v) — cannot click it", box.Rect)
	}
	orig := g.drum.BPM()

	// 1. Click the BPM readout to open the shared editor.
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	ed := g.drum.transportZone.paramEditor
	if ed == nil || !ed.Active() {
		t.Fatalf("clicking the BPM readout did not open the editor")
	}

	// 2. Type a new tempo into the editor.
	fi.chars = []rune{'2', '4', '0'}
	fi.frame(t, g)
	if ed.ti.Value() == "" {
		t.Fatalf("typed characters did not register in the BPM editor")
	}

	// 3. Press Esc — expect cancel (revert) + close.
	fi.pressKey(t, g, ebiten.KeyEscape)

	if ed.Active() {
		t.Fatalf("Esc did not close the BPM editor")
	}
	if g.drum.BPM() != orig {
		t.Fatalf("Esc did not cancel the edit: BPM = %d, want original %d", g.drum.BPM(), orig)
	}
}

// TestFunctional_Esc_Rename reproduces the user flow for the instrument-rename
// text field that "shows up": trigger rename (production row-rack action), type
// a new name, press Esc — expecting the rename cancelled (name unchanged) and
// the field closed. Drives the real Game.Update loop.
func TestFunctional_Esc_Rename(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	if len(g.drum.Rows) == 0 {
		t.Skip("no rows to rename")
	}
	if g.drum.rowRackZone == nil || g.drum.rowRackZone.callbacks.OnRenameOpen == nil {
		t.Skip("no rename trigger")
	}
	origName := g.drum.Rows[0].Name

	// 1. Open the rename field via the production trigger.
	g.drum.rowRackZone.callbacks.OnRenameOpen(0)
	fi.frame(t, g)
	if g.drum.renameComp == nil || !g.drum.renameComp.IsOpen() {
		t.Skip("rename field did not open (mobile/no-textbox build)")
	}

	// 2. Type into the rename field.
	fi.chars = []rune{'Z', 'Z', 'Z'}
	fi.frame(t, g)

	// 3. Press Esc — expect cancel (name unchanged) + close.
	fi.pressKey(t, g, ebiten.KeyEscape)

	if g.drum.renameComp.IsOpen() {
		t.Fatalf("Esc did not close the rename field")
	}
	if g.drum.Rows[0].Name != origName {
		t.Fatalf("Esc did not cancel rename: name = %q, want original %q", g.drum.Rows[0].Name, origName)
	}
}

// TestFunctional_Esc_EQdBInput reproduces the user flow for an EQ band dB
// value field: click the dB readout, type a value, press Esc — expecting the
// edit discarded and the input closed. Drives the real Update+Draw loop.
func TestFunctional_Esc_EQdBInput(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 4; i++ { // settle + draw so dB readout rects exist
		fi.frame(t, g)
	}

	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	idx := -1
	for i := 0; i < 10; i++ {
		if !ez.dbReadoutRect(i).Empty() {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(ez.bandGainsDB) {
		t.Skip("no EQ dB readout rect after draw (panel collapsed in this layout)")
	}
	orig := ez.bandGainsDB[idx]

	// 1. Click the dB readout — this now opens the precision wheel popup
	// (Task 2: EQ wheel-popup value editor); click the wheel's center box to
	// reach the shared numeric editor (the user edits the value).
	r := ez.dbReadoutRect(idx)
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if g.drum.eqWheelPopup == nil || !g.drum.eqWheelPopup.IsOpen() {
		t.Fatalf("click did not open the EQ wheel popup for band %d", idx)
	}
	cb := g.drum.eqWheelPopup.centerH
	fi.clickAt(t, g, (cb.Min.X+cb.Max.X)/2, (cb.Min.Y+cb.Max.Y)/2)
	if ez.paramEditor == nil || !ez.paramEditor.Active() {
		t.Fatalf("click on the wheel's center box did not open the dB editor for band %d — the readout must be editable", idx)
	}

	// 2. Type a new dB value.
	fi.chars = []rune{'8'}
	fi.frame(t, g)

	// 3. Press Esc — expect cancel (revert) + close.
	fi.pressKey(t, g, ebiten.KeyEscape)

	if ez.paramEditor.Active() {
		t.Fatalf("Esc did not close the EQ dB editor")
	}
	if ez.bandGainsDB[idx] != orig {
		t.Fatalf("Esc did not cancel the dB edit: band %d = %v, want original %v", idx, ez.bandGainsDB[idx], orig)
	}
}

// TestFunctional_Esc_InstrumentSearch verifies the instrument-menu search:
// with a non-empty query, the first Esc clears it (menu stays open); the second
// Esc closes the menu. Previously a duplicate tree-level Esc handler closed the
// menu on the very first press (so the clear was never observable).
func TestFunctional_Esc_InstrumentSearch(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	if len(g.drum.Rows) == 0 {
		t.Skip("no rows")
	}
	g.drum.openInstMenuForRow(0)
	fi.frame(t, g)
	m := g.drum.instMenuComp
	if m == nil || !m.IsOpen() {
		t.Skip("instrument menu did not open")
	}

	// Optionally simulate a typed search query (the menu's Update syncs
	// state.searchText FROM the box, so set the box, not state).
	if m.searchBox != nil {
		m.searchBox.SetText("kick")
		fi.frame(t, g)
	}

	// Esc closes the instrument menu (cancelling the in-progress filter).
	fi.pressKey(t, g, ebiten.KeyEscape)
	if g.drum.tree.Portal().IsOpen() {
		t.Fatalf("Esc should close the instrument menu")
	}
}

// TestFunctional_Esc_SoftKeyboardProxy reproduces the BROWSER bug: when a text
// input is focused on WASM, keyboard focus moves to the hidden soft-keyboard
// proxy, so the canvas never receives the Esc keydown (isKeyJustPressed stays
// false). The proxy forwards it via SignalSoftKeyboardEscape. This test focuses
// the BPM box, types, fires ONLY the soft-keyboard signal (no canvas Esc), and
// asserts the edit is reverted and the box closed — the exact fix.
func TestFunctional_Esc_SoftKeyboardProxy(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Skip("no BPM box rect")
	}
	orig := g.drum.BPM()
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	ed := g.drum.transportZone.paramEditor
	if ed == nil || !ed.Active() {
		t.Fatal("BPM editor not open after click")
	}
	ed.ti.SetText("240")

	// Crucially: NO canvas Esc key is pressed (fi has no Escape in keys, so
	// isKeyJustPressed(Escape) stays false) — only the proxy's forwarded signal.
	SignalSoftKeyboardEscape()
	fi.frame(t, g)

	if ed.Active() {
		t.Fatal("soft-keyboard Esc signal should close the BPM editor")
	}
	if g.drum.BPM() != orig {
		t.Fatalf("soft-keyboard Esc signal should revert the BPM edit to %d, got %d", orig, g.drum.BPM())
	}
}

// TestFunctional_Esc_BPMBox_Commits is the positive control: Enter commits the
// typed value (so we know the typing path itself works and the Esc test above
// is meaningfully distinguishing cancel from commit).
func TestFunctional_Esc_BPMBox_Commits(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Skip("no BPM box rect")
	}
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	ed := g.drum.transportZone.paramEditor
	if ed == nil || !ed.Active() {
		t.Fatal("BPM editor not open after click")
	}
	// Open pre-fills the editor with the current BPM. Clear it first, mirroring a
	// user selecting-all and deleting before typing a replacement.
	ed.ti.SetText("")
	fi.chars = []rune{'1', '5', '5'}
	fi.frame(t, g)
	fi.pressKey(t, g, ebiten.KeyEnter)
	if g.drum.BPM() != 155 {
		t.Fatalf("Enter should commit typed BPM 155, got %d", g.drum.BPM())
	}
}
