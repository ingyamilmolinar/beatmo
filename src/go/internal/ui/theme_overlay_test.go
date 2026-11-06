//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func openThemeOverlayForTest(t *testing.T, w, h int) (*Game, *ThemeOverlay, func(x, y, hold int)) {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.Layout(w, h)
	advanceFrames(g, 2)
	g.toggleThemeOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay)
	if !ok || ov == nil {
		t.Fatal("theme overlay is not the top portal overlay after toggle")
	}
	return g, ov, makeHoldTap(g, w, h)
}

func TestThemeOverlayLayoutHasEveryControl(t *testing.T) {
	assertDefaultParityState(t)
	g, ov, _ := openThemeOverlayForTest(t, 1280, 720)
	win := image.Rect(0, 0, g.winW, g.winH)
	if ov.Rect().Empty() || !ov.Rect().In(win) {
		t.Fatalf("panel %v must sit inside the window %v", ov.Rect(), win)
	}
	if ov.Rect().Min.Y < g.gridThemeButtonRect().Max.Y {
		t.Fatalf("panel %v must drop below the theme button %v", ov.Rect(), g.gridThemeButtonRect())
	}
	assertThemeRowsReachable(t, ov)
	mine, circuit := ov.ScopePillRects()
	if mine.Empty() || circuit.Empty() || mine.Min.Y != circuit.Min.Y || mine.Dy() != circuit.Dy() {
		t.Fatalf("scope pills: %v %v", mine, circuit)
	}
	if ov.OverrideRect().Empty() || ov.ResetAllRect().Empty() {
		t.Fatal("override toggle / reset-all missing")
	}
	if ov.Card() == nil || ov.Card().Panel() != ov.Rect() {
		t.Fatal("overlay must be framed by the shared MenuCard")
	}
	if desktopCloseVisible() && (ov.Card().CloseBtn() == nil || !ov.Card().CloseBtn().Rect().In(ov.Rect())) {
		t.Fatal("desktop theme overlay must carry the card's close ×")
	}
	for _, p := range themePresets {
		if ov.PresetRowRect(p.ID).Empty() {
			t.Fatalf("preset %s has no row", p.ID)
		}
	}
	if !ov.AdjustHeaderRect().Overlaps(ov.rowsView) && !ov.scroll.HasScroll() {
		t.Fatal("Adjust header must be reachable")
	}
	for _, c := range themeCategories {
		if !ov.ChipRect(c.ID).Empty() || !ov.ResetRect(c.ID).Empty() {
			t.Fatalf("category %s chips must be hidden while Adjust is collapsed", c.ID)
		}
	}
	ov.SetAdjustExpanded(true)
	advanceFrames(g, 1)
	for _, c := range themeCategories {
		chip, reset := ov.ChipRect(c.ID), ov.ResetRect(c.ID)
		if chip.Empty() || reset.Empty() {
			t.Fatalf("category %s: chip=%v reset=%v", c.ID, chip, reset)
		}
		if chip.Dx() != chip.Dy() {
			t.Fatalf("category %s: chip must be square: %v", c.ID, chip)
		}
		if chip.Overlaps(ov.rowsView) && !chip.In(ov.Rect()) {
			t.Fatalf("category %s: visible chip %v outside panel %v", c.ID, chip, ov.Rect())
		}
		if chip.Overlaps(ov.ResetAllRect()) && chip.Overlaps(ov.rowsView) {
			t.Fatalf("category %s: chip %v collides with Reset all %v", c.ID, chip, ov.ResetAllRect())
		}
	}
	if ov.Rect().Overlaps(g.gridTopButtonsReservedRect()) {
		t.Fatalf("panel %v invades the corner-button cluster %v", ov.Rect(), g.gridTopButtonsReservedRect())
	}
}

func TestThemeOverlayScopeSwitchAndOverrideRealClicks(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	if ov.Scope() != hooks.ThemeScopeUser {
		t.Fatalf("default scope = %q, want user", ov.Scope())
	}
	_, circuit := ov.ScopePillRects()
	cx, cy := center(circuit)
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if ov.Scope() != hooks.ThemeScopeCircuit {
		t.Fatalf("click on 'This circuit' did not switch scope: %q", ov.Scope())
	}
	cx, cy = center(ov.OverrideRect())
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if !g.UserTheme().OverrideCircuits {
		t.Fatal("override toggle click did not flip the flag")
	}
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if g.UserTheme().OverrideCircuits {
		t.Fatal("second click did not flip it back")
	}
}

// expandThemeAdjust taps the "Adjust colors" header (real input) so the
// category chip rows exist, then settles a frame.
func expandThemeAdjust(t *testing.T, g *Game, ov *ThemeOverlay, tap func(x, y, hold int)) {
	t.Helper()
	if ov.AdjustExpanded() {
		return
	}
	scrollThemeHeaderIntoView(t, g, ov)
	cx, cy := center(ov.AdjustHeaderRect())
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if !ov.AdjustExpanded() {
		t.Fatalf("real click on the Adjust header at (%d,%d) did not expand the chips", cx, cy)
	}
}

// scrollThemeHeaderIntoView wheels until the Adjust header row is fully visible.
func scrollThemeHeaderIntoView(t *testing.T, g *Game, ov *ThemeOverlay) {
	t.Helper()
	for i := 0; i < 60 && !ov.AdjustHeaderRect().In(ov.rowsView); i++ {
		ov.scroll.HandleWheel(-1)
		ov.layoutRows()
	}
	advanceFrames(g, 1)
	if !ov.AdjustHeaderRect().In(ov.rowsView) {
		t.Fatalf("could not scroll the Adjust header into view: %v vs %v", ov.AdjustHeaderRect(), ov.rowsView)
	}
}

// scrollThemeRowIntoView wheels the rows until the category's chip is fully
// inside the panel, then settles a frame so the portal republishes hit areas.
func scrollThemeRowIntoView(t *testing.T, g *Game, ov *ThemeOverlay, id string) {
	t.Helper()
	for i := 0; i < 60 && !ov.ChipRect(id).In(ov.rowsView); i++ {
		ov.scroll.HandleWheel(-1)
		ov.layoutRows()
	}
	advanceFrames(g, 1)
	if !ov.ChipRect(id).In(ov.rowsView) {
		t.Fatalf("could not scroll row %s into view: chip %v panel %v", id, ov.ChipRect(id), ov.Rect())
	}
}

func TestThemeOverlayResetButtonsRealClicks(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	g.setUserThemeCategory("accent", "orchid-300")
	g.setUserThemeCategory("focus", "mint-300")
	advanceFrames(g, 2)
	expandThemeAdjust(t, g, ov, tap)
	scrollThemeRowIntoView(t, g, ov, "accent")
	cx, cy := center(ov.ResetRect("accent"))
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if _, ok := g.UserTheme().Categories["accent"]; ok || genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("per-category reset did not clear accent")
	}
	if g.UserTheme().Categories["focus"] != "mint-300" {
		t.Fatal("per-category reset must not touch other categories")
	}
	cx, cy = center(ov.ResetAllRect())
	tap(cx, cy, 3) // first tap arms
	advanceFrames(g, 2)
	if len(g.UserTheme().Categories) != 1 {
		t.Fatal("first Reset-all tap must only ARM (two-tap confirm)")
	}
	tap(cx, cy, 3) // second tap fires
	advanceFrames(g, 2)
	if len(g.UserTheme().Categories) != 0 || genColorFocusRing != genColorDefaultByName["focus-ring"] {
		t.Fatal("reset all did not clear the user theme")
	}
}

func TestThemeOverlayChipShowsEffectiveColorAndRing(t *testing.T) {
	g, ov, _ := openThemeOverlayForTest(t, 1280, 720)
	if set, _ := ov.chipState("accent"); set {
		t.Fatal("no pick yet → chip must read as inherited/default")
	}
	g.setUserThemeCategory("accent", "orchid-300")
	if set, _ := ov.chipState("accent"); !set {
		t.Fatal("after a user pick in user scope the chip is explicitly set (ring)")
	}
	orchid, _ := swatchRGBA("orchid-300")
	if ov.chipColor("accent") != orchid || ov.chipSwatch("accent") != orchid {
		t.Fatalf("chip color = %v swatch = %v want %v", ov.chipColor("accent"), ov.chipSwatch("accent"), orchid)
	}
	// A dark category's chip shows the REAL canvas color, not the purple
	// anchor swatch.
	if ov.chipColor("background") != genColorBackground {
		t.Fatalf("background chip = %v want the live background %v", ov.chipColor("background"), genColorBackground)
	}
	// Circuit scope shows the same effective color but as INHERITED (no ring).
	ov.scope = hooks.ThemeScopeCircuit
	if set, inherited := ov.chipState("accent"); set || !inherited {
		t.Fatalf("circuit scope: set=%v inherited=%v, want inherited only", set, inherited)
	}
}

func TestThemeOverlayMobileFitsPane(t *testing.T) {
	setupMobileTest(t, true)
	g, ov, _ := openThemeOverlayForTest(t, 390, 844)
	ov.SetAdjustExpanded(true)
	advanceFrames(g, 1)
	win := image.Rect(0, 0, g.winW, g.winH)
	if !ov.Rect().In(win) {
		t.Fatalf("mobile panel %v must fit the window %v", ov.Rect(), win)
	}
	for _, c := range themeCategories {
		chip := ov.ChipRect(c.ID)
		if chip.Dx() < TouchMinTarget() {
			t.Fatalf("mobile chip %s below touch min: %v", c.ID, chip)
		}
		// Visible rows keep their chip inside the panel; rows past the
		// viewport are reachable by scrolling.
		if chip.Overlaps(ov.rowsView) && !chip.In(ov.Rect()) {
			t.Fatalf("mobile chip %s %v spills outside panel %v", c.ID, chip, ov.Rect())
		}
	}
	assertThemeRowsReachable(t, ov)
}

// assertThemeRowsReachable: the first row is visible at rest and, when the
// rows scroll, scrolling to the end reveals the last row inside the panel.
func assertThemeRowsReachable(t *testing.T, ov *ThemeOverlay) {
	t.Helper()
	first := themePresets[0].ID
	if !ov.PresetRowRect(first).In(ov.rowsView) {
		t.Fatalf("first preset row %v not inside the rows viewport %v", ov.PresetRowRect(first), ov.rowsView)
	}
	if !ov.AdjustExpanded() {
		return
	}
	last := themeCategories[len(themeCategories)-1].ID
	if ov.ChipRect(last).In(ov.rowsView) {
		return // everything fits
	}
	if !ov.scroll.HasScroll() {
		t.Fatalf("last row %v outside the rows viewport %v but rows do not scroll", ov.ChipRect(last), ov.rowsView)
	}
	for i := 0; i < 50 && !ov.ChipRect(last).In(ov.rowsView); i++ {
		ov.scroll.HandleWheel(-3)
		ov.layoutRows()
	}
	if !ov.ChipRect(last).In(ov.rowsView) {
		t.Fatalf("scrolling to the end does not reveal the last row %s: %v vs viewport %v", last, ov.ChipRect(last), ov.rowsView)
	}
}

// TestThemeOverlayPresetRowsRealClicks: the preset list is the primary
// choice — a real click on Ocean applies it, clears customizations and marks
// the row active; the Adjust header toggles the chip rows (and their hit
// areas); a chip pick on top of a preset keeps the preset and reads
// "customized".
func TestThemeOverlayPresetRowsRealClicks(t *testing.T) {
	defer SetThemeSink(nil)
	SetThemeSink(&fakeThemeSink{})
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	if ov.activePresetID() != defaultThemePresetID {
		t.Fatalf("untouched scope must show %s active, got %s", defaultThemePresetID, ov.activePresetID())
	}
	g.setUserThemeCategory("accent", "orchid-300") // a customization the preset pick must clear
	row := ov.PresetRowRect("ocean")
	if !row.In(ov.rowsView) {
		t.Fatalf("ocean row %v not visible in %v", row, ov.rowsView)
	}
	cx, cy := center(row)
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	u := g.UserTheme()
	if u.Preset != "ocean" || len(u.Categories) != 0 {
		t.Fatalf("preset row click: %+v", u)
	}
	tang, _ := swatchRGBA("tangerine-300")
	if genColorPrimary != tang || ov.activePresetID() != "ocean" {
		t.Fatalf("ocean not applied/active: primary=%v active=%s", genColorPrimary, ov.activePresetID())
	}
	// Chips are hidden and unhittable while collapsed.
	for _, a := range ov.HitAreas() {
		if a.Tag == "theme-reset-accent" {
			t.Fatal("chip reset hit area published while Adjust is collapsed")
		}
	}
	expandThemeAdjust(t, g, ov, tap)
	if ov.ChipRect("background").Empty() {
		t.Fatal("chips missing after expanding")
	}
	g.setUserThemeCategory("focus", "mint-300")
	if g.UserTheme().Preset != "ocean" || !ov.customized() {
		t.Fatalf("chip pick must keep the preset and read customized: %+v", g.UserTheme())
	}
	// Collapse again through the header.
	scrollThemeHeaderIntoView(t, g, ov)
	cx, cy = center(ov.AdjustHeaderRect())
	tap(cx, cy, 3)
	advanceFrames(g, 2)
	if ov.AdjustExpanded() || !ov.ChipRect("background").Empty() {
		t.Fatal("second header click must collapse the chips")
	}
}

// TestThemeOverlayOpensOnEffectiveScope pins that the editor opens on the
// document in effect: a circuit theme without a profile override shows the
// circuit scope (and its preset row), otherwise the profile scope.
func TestThemeOverlayOpensOnEffectiveScope(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.Layout(1280, 720)
	if got := NewThemeOverlay(g).Scope(); got != hooks.ThemeScopeUser {
		t.Fatalf("no circuit theme: scope=%q want user", got)
	}
	g.setCircuitThemePreset("ocean")
	ov := NewThemeOverlay(g)
	if got := ov.Scope(); got != hooks.ThemeScopeCircuit {
		t.Fatalf("circuit theme in effect: scope=%q want circuit", got)
	}
	if got := ov.activePresetID(); got != "ocean" {
		t.Fatalf("active preset=%q want ocean", got)
	}
	g.setThemeOverride(true)
	if got := NewThemeOverlay(g).Scope(); got != hooks.ThemeScopeUser {
		t.Fatalf("profile overrides circuits: scope=%q want user", got)
	}
}
