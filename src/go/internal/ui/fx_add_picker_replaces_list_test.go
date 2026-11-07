//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The picker is a sub-page that REPLACES the effect list in the same frame (title, search band, Back, type
// list), so no card control stays clickable under it, and Back returns to the list as it was. Real presses
// through dv.Update.

// Delay and reverb with inline sliders on desktop, past the frames that guard the opening click.
func fxPickerTestPanel(t *testing.T) (*DrumView, string) {
	t.Helper()
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	fxAdvanceFrames(t, dv, 3)
	return dv, string(instID)
}

func fxPressButton(t *testing.T, dv *DrumView, btn *Button, what string) {
	t.Helper()
	if btn == nil {
		t.Fatalf("%s button not found", what)
	}
	r := btn.Rect()
	fxClickAt(t, dv, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	fxReleaseInput(t, dv)
	fxAdvanceFrames(t, dv, 2)
}

// fxOpenPickerViaClick presses "+ Add Effect" and asserts the picker opened.
func fxOpenPickerViaClick(t *testing.T, dv *DrumView) {
	t.Helper()
	// Rows outside the viewport have no button, so scroll the Add row into view.
	if dv.fxScroll != nil {
		dv.fxScroll.ScrollBehavior().VS.First = dv.fxScroll.ScrollBehavior().VS.Total
		dv.buildFXPanel()
		dv.refreshFXPortalHitAreas()
	}
	fxPressButton(t, dv, findFXPanelBtn(dv, "+ Add Effect"), "+ Add Effect")
	if !dv.fxAddMenuOpen {
		t.Fatal("clicking + Add Effect did not open the picker")
	}
}

func fxEffectListControls(dv *DrumView) (toggles, removes, sliders int) {
	for _, btn := range dv.fxPanelBtns {
		if btn == nil {
			continue
		}
		if isToggle, _ := isFXToggleBtn(btn); isToggle {
			toggles++
		}
		if btn.Icon == string(IconClose) && btn != dv.fxPanelCloseButton {
			removes++
		}
	}
	return toggles, removes, len(dv.fxPanelSliders)
}

// No toggles, remove × or sliders exist under the open picker.
func TestFXAddPicker_ReplacesEffectList(t *testing.T) {
	dv, _ := fxPickerTestPanel(t)

	toggles, removes, sliders := fxEffectListControls(dv)
	if toggles != 2 || removes != 2 || sliders == 0 {
		t.Fatalf("precondition: list should show 2 toggles, 2 removes and sliders; got %d/%d/%d", toggles, removes, sliders)
	}

	fxOpenPickerViaClick(t, dv)

	toggles, removes, sliders = fxEffectListControls(dv)
	if toggles != 0 || removes != 0 || sliders != 0 {
		t.Errorf("picker open but the effect list is still built under/around it: toggles=%d removes=%d sliders=%d (want 0/0/0)",
			toggles, removes, sliders)
	}
	if len(dv.fxAddRowBtn) == 0 {
		t.Error("picker open but no effect-type rows were built")
	}
}

// Nothing between the search band and Back.
func TestFXAddPicker_BackRowIsFirstRowUnderSearch(t *testing.T) {
	dv, _ := fxPickerTestPanel(t)
	fxOpenPickerViaClick(t, dv)

	m := fxPanelMetrics()
	panel := dv.fxPanelRect
	search := dv.fxAddSearchRect
	if search.Empty() {
		t.Fatal("picker open but the search band is not laid out")
	}
	if search.Min.Y != panel.Min.Y+m.headerH {
		t.Errorf("search band top = %d, want directly under the title band (%d)", search.Min.Y, panel.Min.Y+m.headerH)
	}
	if dv.fxScrollOffset() != 0 {
		t.Fatalf("picker should open scrolled to the top, offset=%d", dv.fxScrollOffset())
	}
	back := dv.fxAddMenuBackBtn
	if back == nil {
		t.Fatal("Back row not built at the top of the picker")
	}
	// Back's hit rect is the first lineH row inset by SpaceXS.
	wantTop := search.Max.Y + SpaceXS
	if back.Rect().Min.Y != wantTop {
		t.Errorf("Back row top = %d, want %d (first row directly under the search band); something is laid out above it",
			back.Rect().Min.Y, wantTop)
	}
	// Every type row sits below Back.
	for i, b := range dv.fxAddRowBtn {
		if b.Rect().Min.Y < back.Rect().Max.Y {
			t.Errorf("type row %d (%v) is above the Back row (%v)", i, b.Rect(), back.Rect())
		}
	}
}

// Only the picker's own content.
func TestFXAddPicker_ScrollCoversOnlyPickerContent(t *testing.T) {
	dv, _ := fxPickerTestPanel(t)
	fxOpenPickerViaClick(t, dv)

	m := fxPanelMetrics()
	types := dv.fxAddVisibleEffects()
	want := (1 + fxAddCategoryHeaderCount(types) + len(types)) * m.lineH
	got := dv.ensureFXScroll().ScrollBehavior().VS.Total
	if got != want {
		t.Errorf("picker scroll content = %dpx, want %dpx (Back + %d headers + %d types); the effect list is inflating it",
			got, want, fxAddCategoryHeaderCount(types), len(types))
	}
}

// The same horizontal frame, fully covering the list's rect.
func TestFXAddPicker_CoversListFrame(t *testing.T) {
	dv, _ := fxPickerTestPanel(t)
	listRect := dv.fxPanelRect
	fxOpenPickerViaClick(t, dv)
	pick := dv.fxPanelRect
	if pick.Min.X != listRect.Min.X || pick.Max.X != listRect.Max.X {
		t.Errorf("picker moved horizontally: list %v, picker %v", listRect, pick)
	}
	if !listRect.In(pick) {
		t.Errorf("picker %v does not cover the effect list frame %v", pick, listRect)
	}
}

// Draw agrees with build: no card body is painted.
func TestFXAddPicker_DrawPaintsNoEffectCards(t *testing.T) {
	dv, instID := fxPickerTestPanel(t)
	m := fxPanelMetrics()
	// Card height for an enabled effect with inline params (see drawFXPanel).
	cardHeights := map[int]bool{}
	for _, e := range audio.GetInsertEffects(instrument.ID(instID)) {
		h := m.lineH + len(audio.InsertEffectCatalog()[e.Type])*m.paramH - 3
		cardHeights[h] = true
	}

	countCards := func() int {
		n := 0
		prev := drawRoundedRect
		drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
			if filled && c == colSurface2 && cardHeights[r.Dy()] {
				n++
			}
			prev(dst, r, c, radius, filled)
		}
		defer func() { drawRoundedRect = prev }()
		dst := ebiten.NewImage(800, 300)
		dv.drawFXPanel(dst)
		return n
	}

	if got := countCards(); got != 2 {
		t.Fatalf("precondition: list view should paint 2 effect cards, painted %d", got)
	}
	fxOpenPickerViaClick(t, dv)
	if got := countCards(); got != 0 {
		t.Errorf("picker open but %d effect card(s) were painted with it", got)
	}
}

// In the same frame and scroll position it was left at.
func TestFXAddPicker_BackReturnsToList(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()
	instID := dv.Rows[0].Instrument
	for _, et := range audio.EffectTypeOrder() {
		audio.AddInsertEffect(instID, et, nil)
	}
	t.Cleanup(audio.ClearAllInsertEffects)
	dv.OpenFXPanel(0)
	dv.layoutForTest()
	fxAdvanceFrames(t, dv, 3)
	if !dv.fxHasScroll() {
		t.Fatal("precondition: effect list should overflow and scroll")
	}

	fxOpenPickerViaClick(t, dv) // scrolls the list to its bottom first
	if dv.fxScrollOffset() != 0 {
		t.Errorf("picker should open at its own top, offset=%d", dv.fxScrollOffset())
	}
	fxPressButton(t, dv, dv.fxAddMenuBackBtn, "Back")
	if dv.fxAddMenuOpen {
		t.Fatal("Back did not close the picker")
	}
	if !dv.IsFXPanelOpen() {
		t.Fatal("Back closed the whole FX panel instead of returning to the list")
	}
	if findFXPanelBtn(dv, "+ Add Effect") == nil {
		t.Error("after Back the list should be at the scroll position it was left at (bottom, + Add Effect visible)")
	}
	if toggles, _, _ := fxEffectListControls(dv); toggles == 0 {
		t.Error("after Back the effect list controls were not rebuilt")
	}
}

// The mobile bottom sheet, through the full Game.
func TestFXAddPicker_ReplacesEffectListMobile(t *testing.T) {
	setupMobileTest(t, true)
	g := New(game_log.New(testLogOutput(), game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(audio.ClearAllInsertEffects)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Skip("no rows in mobile layout")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.toggleFXPanel(0)
	if toggles, _, _ := fxEffectListControls(dv); toggles != 2 {
		t.Fatalf("precondition: 2 effect toggles on mobile, got %d", toggles)
	}
	addBtn := findFXPanelBtn(dv, "+ Add Effect")
	if addBtn == nil {
		t.Fatal("+ Add Effect not found on mobile")
	}
	addBtn.OnClick()
	suppressClicksUntilRelease = false
	if !dv.fxAddMenuOpen {
		t.Fatal("picker did not open on mobile")
	}
	if toggles, removes, sliders := fxEffectListControls(dv); toggles+removes+sliders != 0 {
		t.Errorf("mobile picker still builds the effect list: toggles=%d removes=%d sliders=%d", toggles, removes, sliders)
	}
	if dv.fxAddMenuBackBtn == nil || dv.fxAddMenuBackBtn.Rect().Min.Y != dv.fxAddSearchRect.Max.Y+SpaceXS {
		t.Errorf("mobile Back row is not the first row under the search band")
	}
}
