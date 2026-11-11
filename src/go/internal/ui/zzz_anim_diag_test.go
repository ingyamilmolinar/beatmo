package ui

import (
	"image"
	"testing"
)

// Per-frame audit of the arm-grow animation: does any control leave the grid,
// invade the preview ("Your sound") pane, overlap a sibling, or move sideways?
func TestDiag_ArmGrowAnimationAudit(t *testing.T) {
	for _, sz := range [][2]int{{560, 820}, {900, 1000}, {1280, 2000}} {
		w, h := sz[0], sz[1]
		g := newViolinMacroGame(t)
		dv := g.drum
		g.Layout(w, h)
		drillMacro(t, g, "brightness")
		g.Layout(w, h)
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		order := dv.macroConceptKnobIdxs("brightness")
		grid := dv.instEditorMacroGrid
		prev := dv.instEditorPreviewRect
		content := grid.ContentRect()
		t.Logf("[%dx%d] content=%v preview=%v cols=%d visRows=%d", w, h, content, prev, grid.Cols(), grid.visRows)

		rest := map[int]image.Rectangle{}
		onWin := []int{}
		for _, k := range order {
			r := dv.instEditorControls[k].Rect()
			rest[k] = r
			if !r.Empty() {
				onWin = append(onWin, k)
			}
		}
		armK := onWin[0]
		dv.armSynthControl(dv.synthTabActiveInstrument(), armK)
		for f := 0; f < 20; f++ {
			if !dv.advanceArmGrow() {
				break
			}
			dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
			for _, k := range order {
				r := dv.instEditorControls[k].Rect()
				if r.Empty() {
					continue
				}
				if !prev.Empty() && r.Overlaps(prev) {
					t.Errorf("[%dx%d] f%d k=%d INVADES PREVIEW pane: r=%v preview=%v", w, h, f, k, r, prev)
				}
				if r.Max.X > content.Max.X+2 || r.Min.X < content.Min.X-2 {
					t.Errorf("[%dx%d] f%d k=%d ESCAPES GRID horizontally: r=%v content=%v", w, h, f, k, r, content)
				}
				if k != armK && r.Min.X != rest[k].Min.X {
					t.Logf("[%dx%d] f%d k=%d SIDEWAYS dX=%+d (rest %v now %v)", w, h, f, k, r.Min.X-rest[k].Min.X, rest[k], r)
				}
			}
			// pairwise overlap
			for i := 0; i < len(order); i++ {
				for j := i + 1; j < len(order); j++ {
					a := dv.instEditorControls[order[i]].Rect()
					b := dv.instEditorControls[order[j]].Rect()
					if a.Empty() || b.Empty() {
						continue
					}
					if a.Overlaps(b) {
						t.Logf("[%dx%d] f%d OVERLAP k=%d %v  vs k=%d %v", w, h, f, order[i], a, order[j], b)
					}
				}
			}
		}
	}
}
