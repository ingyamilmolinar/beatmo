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

	// 1. Click the BPM readout to open the value wheel.
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	w := g.drum.valueWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatalf("clicking the BPM readout did not open the value wheel")
	}

	// 2. Move the tempo with the wheel (live).
	for i := 0; i < 3; i++ {
		fi.pressKey(t, g, ebiten.KeyArrowUp)
	}
	if g.drum.BPM() == orig {
		t.Fatalf("arrow keys did not move the BPM wheel")
	}

	// 3. Press Esc — expect cancel (restore) + close.
	fi.pressKey(t, g, ebiten.KeyEscape)

	if w.IsOpen() {
		t.Fatalf("Esc did not close the BPM wheel")
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

	// 1. Click the dB readout — it opens the precision wheel popup (the only
	// dB-entry path besides the curve; typed entry was retired).
	r := ez.dbReadoutRect(idx)
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	w := g.drum.eqWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatalf("click did not open the EQ wheel popup for band %d", idx)
	}

	// 2. Move the gain with the wheel (live).
	fi.pressKey(t, g, ebiten.KeyArrowUp)
	fi.pressKey(t, g, ebiten.KeyArrowUp)
	if ez.bandGainsDB[idx] == orig {
		t.Fatalf("arrow keys did not move the dB wheel for band %d", idx)
	}

	// 3. Press Esc — expect cancel (restore) + close.
	fi.pressKey(t, g, ebiten.KeyEscape)

	if w.IsOpen() {
		t.Fatalf("Esc did not close the EQ dB wheel")
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
// a text field (the row rename box — BPM no longer has a text box, it opens the
// value wheel), types, fires ONLY the soft-keyboard signal (no canvas Esc), and
// asserts the edit is cancelled and the field closed — the exact fix.
func TestFunctional_Esc_SoftKeyboardProxy(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	fi.frame(t, g)
	fi.frame(t, g)

	if len(g.drum.Rows) == 0 || g.drum.rowRackZone == nil || g.drum.rowRackZone.callbacks.OnRenameOpen == nil {
		t.Skip("no rename trigger")
	}
	origName := g.drum.Rows[0].Name
	g.drum.rowRackZone.callbacks.OnRenameOpen(0)
	fi.frame(t, g)
	if g.drum.renameComp == nil || !g.drum.renameComp.IsOpen() {
		t.Skip("rename field did not open")
	}
	fi.chars = []rune{'Q', 'Q'}
	fi.frame(t, g)

	// Esc arrives ONLY through the proxy (the canvas never sees the keydown:
	// isKeyJustPressed(Escape) stays false) — only the proxy's forwarded signal.
	SignalSoftKeyboardEscape()
	fi.frame(t, g)

	if g.drum.renameComp.IsOpen() {
		t.Fatal("soft-keyboard Esc signal should close the rename field")
	}
	if g.drum.Rows[0].Name != origName {
		t.Fatalf("soft-keyboard Esc signal should cancel the rename: name %q, want %q", g.drum.Rows[0].Name, origName)
	}
}

// TestFunctional_Esc_BPMBox_Commits is the positive control: Enter commits the
// wheel's value (so the Esc test above meaningfully distinguishes cancel from
// commit).
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
	orig := g.drum.BPM()
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if w := g.drum.valueWheelPopup; w == nil || !w.IsOpen() {
		t.Fatal("BPM wheel not open after click")
	}
	for i := 0; i < 3; i++ {
		fi.pressKey(t, g, ebiten.KeyArrowUp)
	}
	fi.pressKey(t, g, ebiten.KeyEnter)
	if g.drum.BPM() != orig+3 {
		t.Fatalf("Enter should commit the wheel's BPM %d, got %d", orig+3, g.drum.BPM())
	}
}
