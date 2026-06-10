//go:build test

package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"
)

// These tests reproduce the screenshot bug: the FM section of the modular
// voice has 12 knobs which used to be crushed into a single horizontal row.
// The fix lays them out via ControlGrid (>=2 per row, scroll on overflow).

// modularSynthGameRealistic opens the modular synth tab at a realistic desktop
// resolution with the FM stage expanded into the detail pane. The shared
// setupModularSynthGame uses 640×480, where the whole audio panel is only
// ~42 px tall — far too short for any knob row. These grid tests need a panel
// with real vertical room.
//
// Synth-tab redesign: only the SELECTED stage lays out its knobs (the detail
// pane). The FM grid tests exercise the FM section's ControlGrid scroll/axis
// mechanics, so FM must be the open stage before its knobs/grid exist. The
// panel claims its expanded height on the Layout AFTER the tab activates, so
// activate the tab first, resize, re-activate, then open FM and re-layout.
func modularSynthGameRealistic(t *testing.T) *Game {
	t.Helper()
	g := setupModularSynthGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// 800px (vs the widest desktop) keeps the detail pane narrow enough that
	// FM's 13 knobs wrap to >=3 rows — the scroll-mechanics tests need at
	// least two off-window rows so First can advance to 2. At full 1280px the
	// pane fits all 13 knobs in 2 rows (Total=2, max First=1), which can't
	// exercise multi-step scroll.
	g.Layout(800, 720)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// Open the FM stage so its detail-pane knobs + ControlGrid are laid out.
	g.drum.setSelectedSynthSection("modular", synthSectionFM)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	return g
}

// fmSection returns the FM section from the active modular synth tab.
func fmSection(t *testing.T, g *Game) synthSection {
	t.Helper()
	for _, s := range g.drum.SynthTabSections() {
		if s.SectionID() == synthSectionFM {
			return s
		}
	}
	t.Fatal("modular synth tab has no FM section")
	return synthSection{}
}

// visibleFMKnobRects returns the on-screen (non-empty) knob rects of the FM
// section, paired with their global knob index.
func visibleFMKnobRects(g *Game) map[int]image.Rectangle {
	out := map[int]image.Rectangle{}
	s := synthSection{}
	for _, sec := range g.drum.SynthTabSections() {
		if sec.SectionID() == synthSectionFM {
			s = sec
		}
	}
	knobs := g.drum.SynthTabKnobs()
	for _, idx := range s.KnobIdxs() {
		if idx < 0 || idx >= len(knobs) {
			continue
		}
		r := knobs[idx].Rect()
		if r.Empty() {
			continue
		}
		out[idx] = r
	}
	return out
}

func TestSynthSection_FMKnobsDoNotOverlap(t *testing.T) {
	g := modularSynthGameRealistic(t)
	s := fmSection(t, g)
	if s.KnobCount() < 6 {
		t.Fatalf("precondition: FM section should have many knobs, got %d", s.KnobCount())
	}
	card := s.Rect()
	rects := visibleFMKnobRects(g)
	if len(rects) == 0 {
		t.Fatal("no visible FM knobs")
	}
	var list []image.Rectangle
	for idx, r := range rects {
		if !r.In(card) {
			t.Errorf("FM knob %d rect %v not inside card %v", idx, r, card)
		}
		list = append(list, r)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[i].Overlaps(list[j]) {
				t.Errorf("FM knobs overlap: %v vs %v", list[i], list[j])
			}
		}
	}
}

func TestSynthSection_FMKnobsAtLeastTwoPerRow(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil {
		t.Fatal("FM section has no ControlGrid")
	}
	if grid.Cols() < 2 {
		t.Fatalf("FM grid Cols()=%d, want >= 2", grid.Cols())
	}
}

func TestSynthSection_FMKnobsAtLeastMinDiameter(t *testing.T) {
	g := modularSynthGameRealistic(t)
	minD := Profile().DensityValues().SynthKnobMin
	for idx, r := range visibleFMKnobRects(g) {
		if r.Dx() < minD {
			t.Errorf("FM knob %d width %d below SynthKnobMin %d (cramped)", idx, r.Dx(), minD)
		}
	}
}

func TestSynthSection_FMScrolls(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	before := indexSet(visibleFMKnobRects(g))

	// Drive the scroll through the published hit-area, exactly as a wheel
	// event over the FM card would.
	tag := "synth-scroll-" + fmt.Sprint(int(synthSectionFM))
	var wheeled bool
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if strings.HasPrefix(h.Tag, "synth-scroll-") && h.Tag == tag {
			cx := (h.Rect.Min.X + h.Rect.Max.X) / 2
			cy := (h.Rect.Min.Y + h.Rect.Max.Y) / 2
			h.Handler.OnWheel(cx, cy, -1)
			wheeled = true
			break
		}
	}
	if !wheeled {
		t.Fatalf("no %q scroll hit-area published for the FM section", tag)
	}
	// Re-layout so the new scroll window is reflected in the knob rects.
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	after := indexSet(visibleFMKnobRects(g))
	if sameSet(before, after) {
		t.Fatalf("FM scroll did not change visible knobs: %v", before)
	}
}

func TestSynthSection_ThumbDragAdvancesOneRowPerStep(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	// Find the FM scrollbar thumb hit-area.
	tag := "synth-scroll-" + fmt.Sprint(int(synthSectionFM))
	var ha *HitArea
	hits := g.drum.eqPanelZone.HitAreas()
	for i := range hits {
		if hits[i].Tag == tag {
			ha = &hits[i]
			break
		}
	}
	if ha == nil {
		t.Fatalf("no %q thumb hit-area published", tag)
	}
	// Press on the thumb centre, then drag downward in fixed pixel steps.
	thumb := grid.Scroll().ThumbRect()
	cx := (ha.Rect.Min.X + ha.Rect.Max.X) / 2
	cy := (thumb.Min.Y + thumb.Max.Y) / 2
	if res := ha.Handler.OnPress(cx, cy); res != InputCaptured {
		t.Fatalf("press on thumb returned %v, want InputCaptured", res)
	}
	if got := grid.Scroll().VS.First; got != 0 {
		t.Fatalf("First=%d at press, want 0", got)
	}
	// One step -> exactly one row; two steps -> exactly two rows.
	ha.Handler.OnDrag(cx, cy+controlGridDragStepPx)
	if got := grid.Scroll().VS.First; got != 1 {
		t.Errorf("after one step drag: First=%d, want 1", got)
	}
	ha.Handler.OnDrag(cx, cy+2*controlGridDragStepPx)
	if got := grid.Scroll().VS.First; got != 2 {
		t.Errorf("after two step drag: First=%d, want 2", got)
	}
	ha.Handler.OnRelease(cx, cy+2*controlGridDragStepPx)
}

func TestSynthSection_WheelThrottledOneRowPerCooldown(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	// Find the FM card-body scroll catch-all (where wheel-over-empty-space lands).
	tag := "synth-scrollbody-" + fmt.Sprint(int(synthSectionFM))
	var ha *HitArea
	hits := g.drum.eqPanelZone.HitAreas()
	for i := range hits {
		if hits[i].Tag == tag {
			ha = &hits[i]
			break
		}
	}
	if ha == nil {
		t.Fatalf("no %q scroll-body hit-area published", tag)
	}
	cx := (ha.Rect.Min.X + ha.Rect.Max.X) / 2
	cy := (ha.Rect.Min.Y + ha.Rect.Max.Y) / 2

	// One notch -> one row.
	ha.Handler.OnWheel(cx, cy, -1)
	if got := grid.Scroll().VS.First; got != 1 {
		t.Fatalf("first notch: First=%d, want 1", got)
	}
	// Rapid extra notches in the same window are throttled (no fly-through).
	ha.Handler.OnWheel(cx, cy, -1)
	ha.Handler.OnWheel(cx, cy, -1)
	if got := grid.Scroll().VS.First; got != 1 {
		t.Errorf("rapid notches flew past cooldown: First=%d, want 1", got)
	}
	// Advancing real frames (the per-frame Synth update ticks the cooldown)
	// re-arms the wheel — proving the throttle is driven by elapsed cycles.
	for i := 0; i <= controlGridScrollCooldownFrames; i++ {
		g.drum.eqPanelZone.Update()
	}
	ha.Handler.OnWheel(cx, cy, -1)
	if got := grid.Scroll().VS.First; got != 2 {
		t.Errorf("after cooldown frames: First=%d, want 2", got)
	}
}

// firstVisibleFMKnob returns the global index of the first on-window FM knob
// and the hit-adapter published for it.
func firstVisibleFMKnob(t *testing.T, g *Game) (int, HitHandler) {
	t.Helper()
	s := fmSection(t, g)
	knobs := g.drum.SynthTabKnobs()
	kIdx := -1
	for _, idx := range s.KnobIdxs() {
		if idx >= 0 && idx < len(knobs) && !knobs[idx].Rect().Empty() {
			kIdx = idx
			break
		}
	}
	if kIdx < 0 {
		t.Fatal("no visible FM knob")
	}
	tag := fmt.Sprintf("synth-knob-%d", kIdx)
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == tag {
			return kIdx, h.Handler
		}
	}
	t.Fatalf("no hit-area %q for FM knob", tag)
	return 0, nil
}

func TestSynthKnob_VerticalDragScrollsInsteadOfAdjusting(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	kIdx, handler := firstVisibleFMKnob(t, g)
	knob := g.drum.SynthTabKnobs()[kIdx]
	rect := knob.Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	valBefore := knob.Value
	firstBefore := grid.Scroll().VS.First
	if handler.OnPress(cx, cy) != InputCaptured {
		t.Fatal("press on knob did not capture")
	}
	// Drag straight DOWN one full step: must scroll the section one row and
	// leave the knob value untouched.
	handler.OnDrag(cx, cy+controlGridDragStepPx)
	if got := grid.Scroll().VS.First; got != firstBefore+1 {
		t.Errorf("vertical drag on knob: First=%d, want %d (should scroll one row)", got, firstBefore+1)
	}
	if knob.Value != valBefore {
		t.Errorf("vertical drag on knob changed its value %v→%v (must only scroll)", valBefore, knob.Value)
	}
	handler.OnRelease(cx, cy+controlGridDragStepPx)
}

func TestSynthKnob_HorizontalDragAdjustsWithoutScrolling(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	kIdx, handler := firstVisibleFMKnob(t, g)
	knob := g.drum.SynthTabKnobs()[kIdx]
	rect := knob.Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	valBefore := knob.Value
	firstBefore := grid.Scroll().VS.First
	if handler.OnPress(cx, cy) != InputCaptured {
		t.Fatal("press on knob did not capture")
	}
	// Drag RIGHT by half the knob's sweep: must adjust the knob and NOT scroll.
	half := knob.dragPixels() / 2
	handler.OnDrag(cx+half, cy)
	if knob.Value <= valBefore {
		t.Errorf("horizontal drag did not increase knob value (%v→%v)", valBefore, knob.Value)
	}
	if got := grid.Scroll().VS.First; got != firstBefore {
		t.Errorf("horizontal drag scrolled the section (First %d→%d); it must not", firstBefore, got)
	}
	handler.OnRelease(cx+half, cy)
}

// TestSynthKnob_WheelOverKnobScrollsNotTurns reproduces the reported bug:
// scrolling the mouse wheel while the cursor is over a knob must scroll the
// section, NOT turn the knob. The wheel is a vertical gesture and only
// horizontal drag may adjust a knob.
func TestSynthKnob_WheelOverKnobScrollsNotTurns(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	kIdx, handler := firstVisibleFMKnob(t, g)
	knob := g.drum.SynthTabKnobs()[kIdx]
	rect := knob.Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	v0 := knob.Value
	first0 := grid.Scroll().VS.First
	handler.OnWheel(cx, cy, -1) // wheel down over the knob
	if knob.Value != v0 {
		t.Errorf("wheel over knob turned it (%v→%v); the wheel must scroll, never adjust a knob", v0, knob.Value)
	}
	if grid.Scroll().VS.First == first0 {
		t.Errorf("wheel over knob did not scroll the section (First stayed %d)", first0)
	}
}

// TestSynthKnob_MostlyVerticalDragOverKnobScrolls verifies that a drag which is
// mostly vertical — even with a little horizontal drift below the threshold —
// scrolls the section and leaves the knob untouched.
func TestSynthKnob_MostlyVerticalDragOverKnobScrolls(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	kIdx, handler := firstVisibleFMKnob(t, g)
	knob := g.drum.SynthTabKnobs()[kIdx]
	rect := knob.Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	v0 := knob.Value
	first0 := grid.Scroll().VS.First
	if handler.OnPress(cx, cy) != InputCaptured {
		t.Fatal("press did not capture")
	}
	// Mostly vertical (one full step down) with a small horizontal wobble.
	handler.OnDrag(cx+3, cy+controlGridDragStepPx)
	if knob.Value != v0 {
		t.Errorf("mostly-vertical drag turned the knob (%v→%v)", v0, knob.Value)
	}
	if grid.Scroll().VS.First == first0 {
		t.Errorf("mostly-vertical drag did not scroll the section (First stayed %d)", first0)
	}
	handler.OnRelease(cx+3, cy+controlGridDragStepPx)
}

func TestSynthSection_OffWindowKnobsNotHittable(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section does not overflow at this layout; nothing off-window")
	}
	// Collect the FM knob indices that are currently off-window (empty rect).
	s := fmSection(t, g)
	knobs := g.drum.SynthTabKnobs()
	offWindow := map[int]bool{}
	for _, idx := range s.KnobIdxs() {
		if idx >= 0 && idx < len(knobs) && knobs[idx].Rect().Empty() {
			offWindow[idx] = true
		}
	}
	if len(offWindow) == 0 {
		t.Fatal("expected some FM knobs off-window when scrolling")
	}
	// None of those indices may have a synth-knob hit area.
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if !strings.HasPrefix(h.Tag, "synth-knob-") {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(h.Tag, "synth-knob-%d", &idx); err != nil {
			continue
		}
		if offWindow[idx] {
			t.Errorf("off-window FM knob %d still has a hit area (%q rect %v)", idx, h.Tag, h.Rect)
		}
	}
}

func TestSynthSection_DenseSectionsFitAllDensities(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		d := d
		t.Run(fmt.Sprint(d), func(t *testing.T) {
			restore := SetDensityForTest(d)
			defer restore()
			g := modularSynthGameRealistic(t)
			s := fmSection(t, g)
			card := s.Rect()
			minD := Profile().DensityValues().SynthKnobMin
			rects := visibleFMKnobRects(g)
			if len(rects) == 0 {
				t.Fatalf("density %v: no visible FM knobs", d)
			}
			var list []image.Rectangle
			for idx, r := range rects {
				if !r.In(card) {
					t.Errorf("density %v: FM knob %d %v escapes card %v", d, idx, r, card)
				}
				if r.Dx() < minD {
					t.Errorf("density %v: FM knob %d width %d < min %d", d, idx, r.Dx(), minD)
				}
				list = append(list, r)
			}
			for i := 0; i < len(list); i++ {
				for j := i + 1; j < len(list); j++ {
					if list[i].Overlaps(list[j]) {
						t.Errorf("density %v: FM knobs overlap %v vs %v", d, list[i], list[j])
					}
				}
			}
		})
	}
}

func indexSet(m map[int]image.Rectangle) map[int]bool {
	out := map[int]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}
