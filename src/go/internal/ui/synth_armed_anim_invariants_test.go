//go:build test

package ui

import (
	"image"
	"testing"
)

// Per-frame invariants of the armed grow/shrink. ControlGrid's off-window rect is also a zero rect at the
// origin, so lerping to or from it sends a control flying to or out of (0,0); real layouts at several sizes,
// some with one visible row so neighbours really go off-window.

// Wide-but-short grids are the worst case: an off-window control with a far-right target would sweep across the
// whole panel from the origin.
var armAnimLayouts = [][2]int{
	{1280, 2000}, {1600, 900}, {1440, 950}, {1280, 900}, {900, 1000}, {700, 900}, {560, 820},
}

// armAnimFrame is one control's state on one animation frame.
type armAnimFrame struct {
	k    int
	rect image.Rectangle
}

// Yields every grow frame with the resting rects and the geometry it must respect.
func runArmGrowFrames(t *testing.T, w, h int, pick func(onWin []int) int,
	fn func(frame int, rects []armAnimFrame, rest map[int]image.Rectangle, content, preview image.Rectangle)) {
	t.Helper()
	g := newViolinMacroGame(t)
	dv := g.drum
	g.Layout(w, h)
	drillMacro(t, g, "brightness")
	g.Layout(w, h)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())

	order := dv.macroConceptKnobIdxs("brightness")
	grid := dv.instEditorMacroGrid
	if grid == nil || grid.Cols() < 1 {
		t.Skipf("[%dx%d] no drilled macro grid", w, h)
	}
	rest := map[int]image.Rectangle{}
	onWin := []int{}
	for _, k := range order {
		r := dv.instEditorControls[k].Rect()
		rest[k] = r
		if !r.Empty() {
			onWin = append(onWin, k)
		}
	}
	if len(onWin) == 0 {
		t.Skipf("[%dx%d] no on-window controls", w, h)
	}
	content := grid.ContentRect()
	preview := dv.instEditorPreviewRect

	dv.armSynthControl(dv.synthTabActiveInstrument(), pick(onWin))
	for f := 0; f < 50 && dv.advanceArmGrow(); f++ {
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		frames := make([]armAnimFrame, 0, len(order))
		for _, k := range order {
			frames = append(frames, armAnimFrame{k: k, rect: dv.instEditorControls[k].Rect()})
		}
		fn(f, frames, rest, content, preview)
	}
}

// Never outside the grid's content column, which both origin bugs escape.
func TestArmGrowAnim_NeverEscapesGridHorizontally(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		runArmGrowFrames(t, w, h, func(on []int) int { return on[0] },
			func(f int, frames []armAnimFrame, rest map[int]image.Rectangle, content, _ image.Rectangle) {
				for _, fr := range frames {
					if fr.rect.Empty() {
						continue
					}
					if fr.rect.Min.X < content.Min.X-2 || fr.rect.Max.X > content.Max.X+2 {
						t.Fatalf("[%dx%d] frame %d: control %d escaped the grid column: r=%v content=%v",
							w, h, f, fr.k, fr.rect, content)
					}
				}
			})
	}
}

// The armed card grows DOWN and neighbours reflow DOWN.
func TestArmGrowAnim_NothingMovesUp(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		runArmGrowFrames(t, w, h, func(on []int) int { return on[0] },
			func(f int, frames []armAnimFrame, rest map[int]image.Rectangle, _, _ image.Rectangle) {
				for _, fr := range frames {
					if fr.rect.Empty() {
						continue
					}
					if rr, ok := rest[fr.k]; ok && !rr.Empty() && fr.rect.Min.Y < rr.Min.Y-2 {
						t.Fatalf("[%dx%d] frame %d: control %d moved UP: resting=%v now=%v",
							w, h, f, fr.k, rr, fr.rect)
					}
				}
			})
	}
}

// The reflow only pushes down, so an off-window control never scrolls into view; animating it would start at
// (0,0).
func TestArmGrowAnim_OffWindowControlStaysHidden(t *testing.T) {
	checked := 0
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		runArmGrowFrames(t, w, h, func(on []int) int { return on[0] },
			func(f int, frames []armAnimFrame, rest map[int]image.Rectangle, _, _ image.Rectangle) {
				for _, fr := range frames {
					rr, ok := rest[fr.k]
					if !ok || !rr.Empty() {
						continue
					}
					checked++
					if !fr.rect.Empty() {
						t.Fatalf("[%dx%d] frame %d: control %d was off-window at rest but painted at %v",
							w, h, f, fr.k, fr.rect)
					}
				}
			})
	}
	if checked == 0 {
		t.Skip("no layout in the table left a control off-window at rest — invariant is vacuous")
	}
}

// Nothing reaches into the right-hand graph pane.
func TestArmGrowAnim_NeverPaintsIntoPreviewPane(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		runArmGrowFrames(t, w, h, func(on []int) int { return on[0] },
			func(f int, frames []armAnimFrame, _ map[int]image.Rectangle, _, preview image.Rectangle) {
				if preview.Empty() {
					return
				}
				for _, fr := range frames {
					if fr.rect.Empty() {
						continue
					}
					if fr.rect.Overlaps(preview) {
						t.Fatalf("[%dx%d] frame %d: control %d overlapped the preview pane: r=%v preview=%v",
							w, h, f, fr.k, fr.rect, preview)
					}
				}
			})
	}
}

// Each control stays in its column, so its X is identical on every frame.
func TestArmGrowAnim_NeighboursNeverMoveSideways(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		var armed int
		runArmGrowFrames(t, w, h, func(on []int) int { armed = on[0]; return armed },
			func(f int, frames []armAnimFrame, rest map[int]image.Rectangle, _, _ image.Rectangle) {
				for _, fr := range frames {
					if fr.k == armed || fr.rect.Empty() {
						continue // the armed card itself legitimately widens
					}
					rr, ok := rest[fr.k]
					if !ok || rr.Empty() {
						continue
					}
					if fr.rect.Min.X != rr.Min.X {
						t.Fatalf("[%dx%d] frame %d: control %d moved SIDEWAYS: resting X=%d now X=%d (rest=%v now=%v)",
							w, h, f, fr.k, rr.Min.X, fr.rect.Min.X, rr, fr.rect)
					}
				}
			})
	}
}

// Purely vertical in-column travel keeps this true mid-flight, not just at rest.
func TestArmGrowAnim_NeighboursNeverOverlapEachOther(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		var armed int
		runArmGrowFrames(t, w, h, func(on []int) int { armed = on[0]; return armed },
			func(f int, frames []armAnimFrame, _ map[int]image.Rectangle, _, _ image.Rectangle) {
				for i := 0; i < len(frames); i++ {
					for j := i + 1; j < len(frames); j++ {
						a, b := frames[i], frames[j]
						if a.k == armed || b.k == armed || a.rect.Empty() || b.rect.Empty() {
							continue
						}
						if a.rect.Overlaps(b.rect) {
							t.Fatalf("[%dx%d] frame %d: controls %d %v and %d %v overlap",
								w, h, f, a.k, a.rect, b.k, b.rect)
						}
					}
				}
			})
	}
}

// Cut at the fold, never drawn over what follows the grid.
func TestArmGrowAnim_NeverPaintsBelowTheFold(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		g := newViolinMacroGame(t)
		dv := g.drum
		g.Layout(w, h)
		drillMacro(t, g, "brightness")
		g.Layout(w, h)
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		order := dv.macroConceptKnobIdxs("brightness")
		grid := dv.instEditorMacroGrid
		if grid == nil || grid.Cols() < 1 {
			continue
		}
		clip := grid.ClipBottom()
		onWin := []int{}
		for _, k := range order {
			if !dv.instEditorControls[k].Rect().Empty() {
				onWin = append(onWin, k)
			}
		}
		if len(onWin) == 0 {
			continue
		}
		dv.armSynthControl(dv.synthTabActiveInstrument(), onWin[0])
		for f := 0; f < 50 && dv.advanceArmGrow(); f++ {
			dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
			for _, k := range order {
				r := dv.instEditorControls[k].Rect()
				if r.Empty() {
					continue
				}
				if r.Max.Y > clip {
					t.Fatalf("[%dx%d] frame %d: control %d painted below the fold: r=%v clipBottom=%d",
						w, h, f, k, r, clip)
				}
			}
		}
	}
}

// The same reflow with progress running back to 0.
func TestArmGrowAnim_ShrinkObeysSameInvariants(t *testing.T) {
	for _, sz := range armAnimLayouts {
		w, h := sz[0], sz[1]
		g := newViolinMacroGame(t)
		dv := g.drum
		g.Layout(w, h)
		drillMacro(t, g, "brightness")
		g.Layout(w, h)
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		order := dv.macroConceptKnobIdxs("brightness")
		grid := dv.instEditorMacroGrid
		if grid == nil || grid.Cols() < 1 {
			continue
		}
		content := grid.ContentRect()
		preview := dv.instEditorPreviewRect
		rest := map[int]image.Rectangle{}
		onWin := []int{}
		for _, k := range order {
			r := dv.instEditorControls[k].Rect()
			rest[k] = r
			if !r.Empty() {
				onWin = append(onWin, k)
			}
		}
		if len(onWin) == 0 {
			continue
		}
		inst := dv.synthTabActiveInstrument()
		dv.armSynthControl(inst, onWin[0])
		settleArmGrow(dv)
		dv.lockSynthControl(inst)
		for f := 0; f < 50 && dv.advanceArmGrow(); f++ {
			dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
			for _, k := range order {
				r := dv.instEditorControls[k].Rect()
				if r.Empty() {
					continue
				}
				if r.Min.X < content.Min.X-2 || r.Max.X > content.Max.X+2 {
					t.Fatalf("[%dx%d] shrink frame %d: control %d escaped the grid column: r=%v content=%v",
						w, h, f, k, r, content)
				}
				if rr := rest[k]; rr.Empty() {
					t.Fatalf("[%dx%d] shrink frame %d: control %d was off-window at rest but painted at %v",
						w, h, f, k, r)
				} else if r.Min.Y < rr.Min.Y-2 {
					t.Fatalf("[%dx%d] shrink frame %d: control %d moved UP: resting=%v now=%v",
						w, h, f, k, rr, r)
				}
				if !preview.Empty() && r.Overlaps(preview) {
					t.Fatalf("[%dx%d] shrink frame %d: control %d overlapped the preview pane: r=%v preview=%v",
						w, h, f, k, r, preview)
				}
			}
		}
	}
}
