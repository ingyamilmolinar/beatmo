//go:build test

package ui

import (
	"image"
	"testing"
)

// synth_armed_anim_invariants_test.go — per-FRAME geometry invariants for the
// armed-control grow/shrink animation.
//
// Every earlier armed-grow test inspects only the SETTLED frame. That blind spot
// hid two separate origin-anchored bugs, because ControlGrid's "off-window"
// return value (image.Rectangle{}) is simultaneously a NOT-VISIBLE sentinel and
// a zero-size rect at the SCREEN ORIGIN — so feeding it to lerpRect makes a
// control travel to or from (0,0):
//
//   - empty TARGET  (neighbour reflowed onto a row below the window): the control
//     flew UP-LEFT toward (0,0), shrinking to nothing.
//   - empty RESTING (control already off-window before the grow): the control
//     flew OUT of (0,0), growing from a tiny square and sweeping across unrelated
//     chrome — visible as a stray square over the "Your sound" preview pane.
//
// These run the REAL layout path (eqPanelZone.Layout) frame by frame over several
// window sizes, including sizes where the grid has a single visible row so the
// reflow genuinely pushes neighbours off-window.

// armAnimLayouts covers a wide grid (everything stays on-window), WIDE-BUT-SHORT
// grids (many columns AND a single visible row — the worst case: a control that is
// off-window at rest has a far-RIGHT target, so interpolating from the origin
// sentinel sweeps it diagonally across the whole panel, including the right-hand
// preview pane), and narrow grids with one visible row.
var armAnimLayouts = [][2]int{
	{1280, 2000}, {1600, 900}, {1440, 950}, {1280, 900}, {900, 1000}, {700, 900}, {560, 820},
}

// armAnimFrame is one control's state on one animation frame.
type armAnimFrame struct {
	k    int
	rect image.Rectangle
}

// runArmGrowFrames arms `pick` (chosen from the on-window controls) and yields
// every frame of the grow to fn, along with the resting rects and the geometry
// the frame must respect.
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

// No control may ever paint outside the grid's own content column. The empty-
// target bug sent neighbours up-left to (0,0); the empty-resting bug sent them
// out of (0,0). Both escape the grid horizontally, which this catches directly.
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

// The grow must never lift a control above where it was resting: the armed card
// grows DOWN, neighbours reflow DOWN.
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

// A control that is off-window BEFORE the grow must stay hidden throughout it.
// The reflow only pushes controls down, so one can never scroll into view — and
// animating it means interpolating from the empty sentinel at (0,0), which is
// what painted a stray square over the preview pane.
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

// Nothing the grow animates may reach into the right-hand "Your sound" preview
// pane. This is the user-visible symptom the empty-resting bug produced: a small
// square drifting across the preview graphs.
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

// Neighbours move DOWN ONLY — never sideways. reflowArmedAnchored keeps each
// control in its column, so its X is pixel-identical on every frame of the grow.
// (The previous reading-order masonry reassigned columns, so cards flew diagonally
// across the panel.)
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

// Two non-armed cards must never overlap on any frame. Purely vertical, in-column
// travel makes this hold throughout the transition, not just at rest — the old
// masonry violated it mid-flight when a card crossed a stationary sibling.
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

// No control may paint below the grid's fold — a neighbour sliding toward an
// off-window row is cut off at the bottom edge, never drawn over whatever
// follows the grid.
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

// The SHRINK half (disarming) must obey the same rules — it replays the same
// reflow with the progress running back down to 0.
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
