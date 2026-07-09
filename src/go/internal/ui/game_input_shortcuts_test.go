package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestCameraReset(t *testing.T) {
	c := NewCamera()
	c.Scale = 0.37
	c.OffsetX = 123
	c.OffsetY = -456
	c.Reset()
	if c.Scale != 2.0 {
		t.Fatalf("Reset Scale = %v, want 2.0", c.Scale)
	}
	if c.OffsetX != 0 || c.OffsetY != 0 {
		t.Fatalf("Reset Offset = (%v,%v), want (0,0)", c.OffsetX, c.OffsetY)
	}
}

func TestTriggerPlayPauseSetsPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.TriggerPlayPause()
	if !g.drum.PlayPressed() {
		t.Fatal("TriggerPlayPause should make PlayPressed() return true once")
	}
	if g.drum.PlayPressed() {
		t.Fatal("PlayPressed() should be a one-shot (cleared after read)")
	}
}

func TestTriggerStopSetsPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.TriggerStop()
	if !g.drum.StopPressed() {
		t.Fatal("TriggerStop should make StopPressed() return true once")
	}
	if g.drum.StopPressed() {
		t.Fatal("StopPressed() should be a one-shot (cleared after read)")
	}
}

func TestSpaceTogglesPlayPulse(t *testing.T) {
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	defer restore()

	g.handleGlobalShortcuts()
	if !g.drum.PlayPressed() {
		t.Fatal("Space should trigger the play/pause pulse")
	}
}

func TestActionKeysIgnoredWhileTextInputFocused(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.tree.SetFocus("transport")

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	defer restore()

	g.handleGlobalShortcuts()
	if g.drum.PlayPressed() {
		t.Fatal("Space must NOT trigger play while a text input owns the keyboard")
	}
}

func TestArrowKeysPan(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.OffsetX = 0
	g.cam.OffsetY = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.OffsetX <= 0 {
		t.Fatalf("ArrowLeft should increase OffsetX, got %v", g.cam.OffsetX)
	}

	g.cam.OffsetX, g.cam.OffsetY = 0, 0
	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowUp: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.OffsetY <= 0 {
		t.Fatalf("ArrowUp should increase OffsetY, got %v", g.cam.OffsetY)
	}
}

func TestBracketKeysZoom(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 2.0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketRight: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.Scale <= 2.0 {
		t.Fatalf("] should zoom in (increase scale), got %v", g.cam.Scale)
	}
	zoomedIn := g.cam.Scale

	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketLeft: true}, nil)
	g.handleGlobalShortcuts()
	restore()
	if g.cam.Scale >= zoomedIn {
		t.Fatalf("[ should zoom out (decrease scale), got %v", g.cam.Scale)
	}
}

func TestZoomStaysInBounds(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 9.95
	for i := 0; i < 200; i++ {
		restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyBracketRight: true}, nil)
		g.handleGlobalShortcuts()
		restore()
	}
	if g.cam.Scale > 10.0 {
		t.Fatalf("zoom in must clamp at 10.0, got %v", g.cam.Scale)
	}
}

func TestZeroResetsZoom(t *testing.T) {
	g := newTestGameForUndo(t)
	g.cam.Scale = 0.5
	g.cam.OffsetX = 999
	g.cam.OffsetY = -999
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.Key0: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.cam.Scale != 2.0 || g.cam.OffsetX != 0 || g.cam.OffsetY != 0 {
		t.Fatalf("0 should reset camera: scale=%v off=(%v,%v)", g.cam.Scale, g.cam.OffsetX, g.cam.OffsetY)
	}
}

func TestEqualKeyRaisesBPM(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEqual: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start+1 {
		t.Fatalf("'+' should raise BPM by 1: %d -> %d", start, g.drum.BPM())
	}
}

func TestMinusKeyLowersBPM(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyMinus: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start-1 {
		t.Fatalf("'-' should lower BPM by 1: %d -> %d", start, g.drum.BPM())
	}
}

func TestShiftPlusStillRaisesBPMByOne(t *testing.T) {
	// BPM nudge is +/-1 only — Shift no longer multiplies the step.
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyShiftLeft: true},
		map[ebiten.Key]bool{ebiten.KeyEqual: true},
	)
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() != start+1 {
		t.Fatalf("Shift+'+' should still raise BPM by only 1: %d -> %d", start, g.drum.BPM())
	}
}

func TestNumberKeysSwitchTabs(t *testing.T) {
	// Keys map to the on-screen (display) tab order: 1 = leftmost (EQ).
	cases := []struct {
		key ebiten.Key
		tab PanelTab
	}{
		{ebiten.Key1, TabEQ},
		{ebiten.Key2, TabWave},
		{ebiten.Key3, TabSpectrum},
		{ebiten.Key4, TabMeters},
		{ebiten.Key5, TabScope},
		{ebiten.Key6, TabSynth},
		{ebiten.Key7, TabSampler},
	}
	// Guard: the expectations must match the live display order so this test
	// tracks any future reordering of AllPanelTabs().
	for i, c := range cases {
		if AllPanelTabs()[i] != c.tab {
			t.Fatalf("display order drift at index %d: AllPanelTabs()=%v, expected %v", i, AllPanelTabs()[i], c.tab)
		}
	}
	for _, c := range cases {
		g := newTestGameForUndo(t)
		restore := stubKeys(nil, map[ebiten.Key]bool{c.key: true})
		g.handleGlobalShortcuts()
		restore()
		if got := g.drum.eqPanelZone.tabState.ActiveTab(); got != c.tab {
			t.Fatalf("key %v: active tab = %v, want %v", c.key, got, c.tab)
		}
	}
}

func TestEscFallbackStopsPlayback(t *testing.T) {
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if !g.drum.StopPressed() {
		t.Fatal("Esc with nothing pending should request Stop")
	}
}

func TestEscClosesPortalNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.portal().Open(PortalEntry{ID: "test-menu", Overlay: NewTooltipOverlay("x")})
	if !g.drum.portal().IsOpen() {
		t.Fatal("setup: portal should be open")
	}
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.portal().IsOpen() {
		t.Fatal("Esc should close the open portal")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that closed a portal must NOT also Stop")
	}
}

func TestEscCancelsConnectModeNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.connectMode = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.connectMode {
		t.Fatal("Esc should cancel connect mode")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled connect mode must NOT also Stop")
	}
}

func TestEscCancelsMoveModeNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.moveMode = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.moveMode {
		t.Fatal("Esc should cancel move mode")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled move mode must NOT also Stop")
	}
}

func TestEscCancelsLinkDragNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.linkDrag.active = true
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.linkDrag.active {
		t.Fatal("Esc should cancel an in-progress link drag")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that cancelled link drag must NOT also Stop")
	}
}

func TestEscBlursTextInputNotStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.tree.SetFocus("transport")
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.tree.focusedZone != "" {
		t.Fatal("Esc should blur the focused text input")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc that blurred a field must NOT also Stop")
	}
}

func TestSlashTogglesHelpOverlay(t *testing.T) {
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	g.handleGlobalShortcuts()
	restore()
	if !g.drum.portal().IsOpen() {
		t.Fatal("'?' should open the shortcuts overlay")
	}

	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.portal().IsOpen() {
		t.Fatal("'?' again should close the shortcuts overlay")
	}
}

func TestEscRevertsBPMEdit(t *testing.T) {
	g := newTestGameForUndo(t)
	tz := g.drum.transportZone
	if tz == nil || tz.bpmBox == nil {
		t.Skip("no transport bpm box")
	}
	orig := g.drum.BPM()
	// Open the shared editor and type a different (uncommitted) value. Escape is
	// owned by the editor's own Update — it cancels without writing.
	tz.openBPMEditor()
	tz.paramEditor.ti.SetText("240")

	restore := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
	)
	defer restore()
	tz.Update()

	if tz.paramEditor.Active() {
		t.Fatal("Esc should close the BPM editor")
	}
	if g.drum.transportZone.BPM() != orig {
		t.Fatalf("Esc should leave BPM at %d (uncommitted), got %d", orig, g.drum.transportZone.BPM())
	}
}

func TestEscRevertsEQDBEdit(t *testing.T) {
	g := newTestGameForUndo(t)
	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	// Open the shared dB editor for band 0 with a saved prior value of 0.0 and
	// a changed (uncommitted) text. Escape is owned by the editor's own Update.
	ez.bandGainsDB[0] = 0.0
	ez.openEQDBEditor(0)
	ez.paramEditor.ti.SetText("12.0")

	restore := stubKeys(
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
	)
	defer restore()
	ez.Update()

	if ez.paramEditor.Active() {
		t.Fatalf("Esc should close the EQ dB editor")
	}
	if ez.bandGainsDB[0] != 0.0 {
		t.Fatalf("Esc should leave the dB gain at 0.0, got %v", ez.bandGainsDB[0])
	}
}

func TestEscClearsInstrumentSearchThenCloses(t *testing.T) {
	g := newTestGameForUndo(t)
	m := g.drum.instMenuComp
	if m == nil {
		t.Skip("no instrument menu component")
	}
	m.Open()
	g.drum.openInstMenuPortal()
	m.state.searchText = "kick"

	// First Esc clears the search, keeps the menu open.
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	restore()
	if m.state.searchText != "" {
		t.Fatalf("first Esc should clear search, got %q", m.state.searchText)
	}
	if !g.drum.portal().IsOpen() {
		t.Fatal("menu should stay open after clearing search")
	}

	// Second Esc closes the menu.
	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.portal().IsOpen() {
		t.Fatal("second Esc should close the menu")
	}
}

func TestEscCancelsRename(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	cancelled := false
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, OnCancel: func() { cancelled = true }})
	rc.Open()
	g.drum.openRenamePortal()
	if !g.drum.portal().IsOpen() {
		t.Fatal("setup: rename portal should be open")
	}

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleGlobalShortcuts()

	if !cancelled {
		t.Fatal("Esc should run the rename OnCancel")
	}
	if g.drum.portal().IsOpen() {
		t.Fatal("Esc should close the rename portal")
	}
}

func TestToggleSettingsOverlayDirect(t *testing.T) {
	g := newTestGameForUndo(t)
	g.toggleSettingsOverlay()
	if !g.drum.portal().IsOpen() {
		t.Fatal("toggleSettingsOverlay should open the portal")
	}
	g.toggleSettingsOverlay()
	if g.drum.portal().IsOpen() {
		t.Fatal("toggleSettingsOverlay should close the portal")
	}
}

func TestSlashIgnoredWhileTextInputFocused(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.tree.SetFocus("transport")
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.portal().IsOpen() {
		t.Fatal("'/' must NOT open help while a text input owns the keyboard (it should type into the field)")
	}
}

func TestNumpadAdjustsBPM(t *testing.T) {
	g := newTestGameForUndo(t)
	start := g.drum.BPM()
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyNumpadAdd: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.BPM() != start+1 {
		t.Fatalf("numpad + should raise BPM by 1: %d -> %d", start, g.drum.BPM())
	}
	cur := g.drum.BPM()
	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyNumpadSubtract: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.BPM() != cur-1 {
		t.Fatalf("numpad - should lower BPM by 1: %d -> %d", cur, g.drum.BPM())
	}
}

func TestBPMNudgeClampsAtMinimum(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.SetBPM(1)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyMinus: true})
	defer restore()
	g.handleGlobalShortcuts()
	if g.drum.BPM() < 1 {
		t.Fatalf("BPM must clamp at >= 1, got %d", g.drum.BPM())
	}
}

func TestGridHelpButtonExistsTopRight(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.gridHelpBtn == nil {
		t.Fatal("grid help button should be constructed")
	}
	r := g.gridHelpButtonRect()
	if r.Empty() {
		t.Fatal("grid help button rect should be non-empty on desktop")
	}
	gr := g.split.GridRect(g.winW, g.winH)
	// True top-right corner of the grid pane: right edge near gridRect.Max.X,
	// top near gridRect.Min.Y (NOT the +gridTopOffset content origin).
	if r.Max.X > gr.Max.X || r.Min.X < gr.Max.X-80 {
		t.Fatalf("button should hug the right edge of the grid (max.X=%d), got %v", gr.Max.X, r)
	}
	if r.Min.Y < gr.Min.Y || r.Min.Y > gr.Min.Y+40 {
		t.Fatalf("button should sit in the top corner (gridTop=%d), got %v", gr.Min.Y, r)
	}
}

func TestGridHelpButtonTogglesOverlay(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.gridHelpBtn == nil {
		t.Fatal("grid help button should be constructed")
	}
	r := g.gridHelpButtonRect()
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Press inside the button → opens the overlay (Button fires OnClick on press edge).
	if g.gridHelpBtn.HandleInputResult(cx, cy, true) != InputConsumed {
		t.Fatal("pressing the grid help button should consume the input")
	}
	if !g.drum.portal().IsOpen() {
		t.Fatal("pressing the grid help button should open the shortcuts overlay")
	}
	// Release, then press again → closes it.
	g.gridHelpBtn.HandleInputResult(cx, cy, false)
	g.gridHelpBtn.HandleInputResult(cx, cy, true)
	if g.drum.portal().IsOpen() {
		t.Fatal("pressing again should close the shortcuts overlay")
	}
}

func TestGridHelpButtonBlocksGridEditAndPan(t *testing.T) {
	g := newTestGameForUndo(t)
	r := g.gridHelpButtonRect()
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if !g.blocksAt(cx, cy) {
		t.Fatal("blocksAt should be true over the help button (no node-create)")
	}
	if !g.menuHit(cx, cy) {
		t.Fatal("menuHit should be true over the help button (no camera pan)")
	}
}

func TestArrowKeysYieldToSynthValueEditor(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.paramEditor = NewParamValueEditor()
	g.drum.paramEditor.Open(audio.ParamDef{Name: "g", Min: 0, Max: 1000}, image.Rect(0, 0, 80, 24),
		func() float64 { return 120 }, func(float64) {})
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowLeft must NOT pan the grid while the synth/sampler value editor is active (OffsetX=%v); keys must reach the editor caret", g.cam.OffsetX)
	}
}

func TestArrowKeysYieldToEQValueEditor(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.drum.eqPanelZone == nil {
		t.Skip("no eq panel zone")
	}
	if g.drum.eqPanelZone.paramEditor == nil {
		g.drum.eqPanelZone.paramEditor = NewParamValueEditor()
	}
	g.drum.eqPanelZone.openEQDBEditor(0)
	g.cam.OffsetX = 0

	restore := stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowRight: true}, nil)
	defer restore()
	g.handleGlobalShortcuts()

	if g.cam.OffsetX != 0 {
		t.Fatalf("ArrowRight must NOT pan the grid while the EQ dB editor is active (OffsetX=%v)", g.cam.OffsetX)
	}
}
