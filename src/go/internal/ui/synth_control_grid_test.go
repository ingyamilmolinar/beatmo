//go:build test

package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// FM's 12+ knobs lay out through ControlGrid (≥2 per row, scrolling on overflow), not one crushed row.

// At 640×480 the panel is ~42 px, too short for a knob row. Only the selected stage lays out knobs, and the panel
// claims its height on the Layout after the tab activates: activate, resize, re-activate, open FM, lay out.
func modularSynthGameRealistic(t *testing.T) *Game {
	t.Helper()
	g := setupModularSynthGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// Narrow enough that FM wraps to ≥3 rows, so First can advance twice; at 1280 it fits two.
	g.Layout(800, 720)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// With More open, so the fine-tuning operators lay out too.
	g.drum.setSelectedSynthSection(roster.Modular, synthSectionFM)
	g.drum.setSynthMoreOpen(roster.Modular, true)
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

// Keyed by global knob index.
func visibleFMKnobRects(g *Game) map[int]image.Rectangle {
	out := map[int]image.Rectangle{}
	s := synthSection{}
	for _, sec := range g.drum.SynthTabSections() {
		if sec.SectionID() == synthSectionFM {
			s = sec
		}
	}
	knobs := g.drum.SynthTabControls()
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

func TestSynthSection_FMControlsDoNotOverlap(t *testing.T) {
	g := modularSynthGameRealistic(t)
	s := fmSection(t, g)
	if s.KnobCount() < 6 {
		t.Fatalf("precondition: FM section should have many knobs, got %d", s.KnobCount())
	}
	card := g.drum.instEditorMacroDetailR
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

func TestSynthSection_FMControlsAtLeastTwoPerRow(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil {
		t.Fatal("FM section has no ControlGrid")
	}
	if grid.Cols() < 2 {
		t.Fatalf("FM grid Cols()=%d, want >= 2", grid.Cols())
	}
}

func TestSynthSection_FMControlsAtLeastMinDiameter(t *testing.T) {
	g := modularSynthGameRealistic(t)
	minD := Profile().DensityValues().SynthControlMin
	for idx, r := range visibleFMKnobRects(g) {
		if r.Dx() < minD {
			t.Errorf("FM knob %d width %d below SynthControlMin %d (cramped)", idx, r.Dx(), minD)
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

	// Through the published hit area, as a wheel over the card.
	tag := "synth-macro-scroll"
	var wheeled bool
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == tag {
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
	tag := "synth-macro-scroll"
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

// The surface a finger grabs to scroll, as opposed to the thumb.
func synthBodyHitArea(t *testing.T, g *Game) *HitArea {
	t.Helper()
	tag := "synth-macro-scrollbody"
	hits := g.drum.eqPanelZone.HitAreas()
	for i := range hits {
		if hits[i].Tag == tag {
			return &hits[i]
		}
	}
	t.Fatalf("no %q scroll-body hit-area published", tag)
	return nil
}

// Grabbing the card body scrolls naturally: the content follows the finger, unlike the direct thumb. The body
// rides the smooth pixel + momentum path with a dead zone and a direction lock, so several drag steps run and
// only the direction is asserted.
func TestSynthSection_BodyDragIsNatural(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	ha := synthBodyHitArea(t, g)
	cx := (ha.Rect.Min.X + ha.Rect.Max.X) / 2
	cy := (ha.Rect.Min.Y + ha.Rect.Max.Y) / 2

	// Finger up in 20 px steps: later rows, First rises.
	grid.Scroll().VS.First = 1
	if res := ha.Handler.OnPress(cx, cy); res != InputCaptured {
		t.Fatalf("press on body returned %v, want InputCaptured", res)
	}
	y := cy
	for i := 0; i < 10; i++ {
		y -= 20
		ha.Handler.OnDrag(cx, y)
	}
	ha.Handler.OnRelease(cx, y)
	if got := grid.Scroll().VS.First; got <= 1 {
		t.Errorf("finger UP (smooth natural drag): First=%d, want > 1 (drag up should reveal later rows)", got)
	}

	// Finger down: earlier rows, First falls.
	grid.Scroll().VS.First = 1
	if res := ha.Handler.OnPress(cx, cy); res != InputCaptured {
		t.Fatalf("press on body returned %v, want InputCaptured", res)
	}
	y = cy
	for i := 0; i < 10; i++ {
		y += 20
		ha.Handler.OnDrag(cx, y)
	}
	ha.Handler.OnRelease(cx, y)
	if got := grid.Scroll().VS.First; got >= 1 {
		t.Errorf("finger DOWN (smooth natural drag): First=%d, want < 1 (drag down should reveal earlier rows)", got)
	}
}

// A smooth section's wheel moves in proportion to the notches with no cooldown; the stepped contract is
// TestControlGrid_DefaultStaysStepped's.
func TestSynthSection_WheelIsContinuous(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Fatalf("FM section should overflow and scroll (grid=%v)", grid)
	}
	// Find the FM card-body scroll catch-all (where wheel-over-empty-space lands).
	tag := "synth-macro-scrollbody"
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

	// A 3-notch spin moves more than one row.
	first := grid.Scroll().VS.First
	ha.Handler.OnWheel(cx, cy, -3)
	if got := grid.Scroll().VS.First; got < first+2 {
		t.Fatalf("multi-notch wheel over smooth synth section did not advance continuously: First=%d, want >= %d", got, first+2)
	}

	// Back-to-back notches move at once.
	afterSpin := grid.Scroll().VS.First
	ha.Handler.OnWheel(cx, cy, -1)
	if got := grid.Scroll().VS.First; got <= afterSpin {
		t.Fatalf("back-to-back wheel notch was throttled on a smooth synth section: First=%d, want > %d", got, afterSpin)
	}
}

// With the hit adapter published for it.
func firstVisibleFMKnob(t *testing.T, g *Game) (int, HitHandler) {
	t.Helper()
	s := fmSection(t, g)
	knobs := g.drum.SynthTabControls()
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
	tag := fmt.Sprintf("synth-control-%d", kIdx)
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if h.Tag == tag {
			return kIdx, h.Handler
		}
	}
	t.Fatalf("no hit-area %q for FM knob", tag)
	return 0, nil
}

func TestSynthSection_OffWindowControlsNotHittable(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section does not overflow at this layout; nothing off-window")
	}
	// Collect the FM knob indices that are currently off-window (empty rect).
	s := fmSection(t, g)
	knobs := g.drum.SynthTabControls()
	offWindow := map[int]bool{}
	for _, idx := range s.KnobIdxs() {
		if idx >= 0 && idx < len(knobs) && knobs[idx].Rect().Empty() {
			offWindow[idx] = true
		}
	}
	if len(offWindow) == 0 {
		t.Fatal("expected some FM knobs off-window when scrolling")
	}
	// None of those indices may have a synth-control hit area.
	for _, h := range g.drum.eqPanelZone.HitAreas() {
		if !strings.HasPrefix(h.Tag, "synth-control-") {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(h.Tag, "synth-control-%d", &idx); err != nil {
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
			_ = fmSection(t, g)
			card := g.drum.instEditorMacroDetailR
			minD := Profile().DensityValues().SynthControlMin
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
