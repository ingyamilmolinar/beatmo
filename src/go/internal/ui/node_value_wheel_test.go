//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Node sidebar values (spec §3): tapping a value pill (volume, duration, logic
// N, logic P, groove %) opens the shared value wheel. Each detent writes the
// node LIVE (predictor sees it); closing records ONE undo step; Esc restores.
// The ± steppers keep working and share one limits table with the wheel and the
// JS bridge.

func newNodeWheelGame(t *testing.T, setup func(p *model.NodeParams), sections ...sbSection) (*Game, *uiNode, func(x, y, frames int)) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add a node")
	}
	if setup != nil {
		p := nodeParams(g, n.ID)
		setup(&p)
		g.graph.SetNodeParams(n.ID, p)
	}
	g.updateBeatInfos()
	g.sidebar.Open(n)
	if len(sections) == 0 {
		g.sidebar.ExpandAllSections()
	}
	for _, sec := range sections { // only these (keeps a lower section inside the viewport)
		g.sidebar.sectionOpen[sec] = true
	}
	screen := ebiten.NewImage(1280, 720)
	for i := 0; i < 3; i++ {
		g.Draw(screen)
		advanceFrames(g, 1)
	}
	return g, n, makeHoldTap(g, 1280, 720)
}

func nodeParams(g *Game, id model.NodeID) model.NodeParams {
	mn, _ := g.graph.GetNodeByID(id)
	return mn.Params
}

func openNodeWheelByTap(t *testing.T, g *Game, tap func(x, y, frames int), pill sbID) {
	t.Helper()
	r, ok := g.sidebar.rects[pill]
	if !ok || r.Empty() {
		t.Fatalf("sidebar pill %q has no rect", pill)
	}
	x, y := center(r)
	tap(x, y, 3)
	advanceFrames(g, 2)
	if !g.drum.valueWheelPopup.IsOpen() {
		t.Fatalf("tapping %q must open the value wheel", pill)
	}
}

func TestNodeVolumeWheelLiveAndOneCommit(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, nil)
	v0 := nodeParams(g, n.ID).Volume
	undo0 := len(g.undoManager.undo)
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)
	v1 := nodeParams(g, n.ID).Volume
	if v1 >= v0 {
		t.Fatalf("two detents down must lower the volume live: %v → %v", v0, v1)
	}
	if got := 20 * math.Log10(v1/v0); math.Abs(got+2) > 0.01 {
		t.Errorf("two 1 dB detents = %.3f dB, want -2", got)
	}
	if len(g.undoManager.undo) != undo0 {
		t.Fatalf("no undo step mid-gesture")
	}
	pressKeyThroughShortcuts(g, ebiten.KeyEnter)
	advanceFrames(g, 1)
	if g.drum.valueWheelPopup.IsOpen() {
		t.Fatal("Enter closes the wheel")
	}
	if n2 := len(g.undoManager.undo); n2 != undo0+1 {
		t.Fatalf("closing records exactly ONE undo step (%d → %d)", undo0, n2)
	}
}

func TestNodeVolumeWheelEscRestores(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, nil)
	v0 := nodeParams(g, n.ID).Volume
	undo0 := len(g.undoManager.undo)
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	advanceFrames(g, 1)
	restore()
	advanceFrames(g, 1)
	if got := nodeParams(g, n.ID).Volume; got != v0 {
		t.Fatalf("Esc must restore the volume %v, got %v", v0, got)
	}
	if len(g.undoManager.undo) != undo0 {
		t.Fatalf("Esc records nothing")
	}
}

func TestNodeVolumeWheelCapsAtPlus12AndMutesAtTheBottom(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, nil)
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	m := g.drum.valueWheelPopup.model().(*NumberModel)
	m.SetLadderIndex(m.LadderLen() - 1) // 6 dB steps
	for i := 0; i < 10; i++ {
		g.drum.valueWheelPopup.StepValue(+1)
	}
	if db := 20 * math.Log10(nodeParams(g, n.ID).Volume); db > 12.0001 {
		t.Fatalf("volume wheel must cap at +12 dB, got %+.2f dB", db)
	}
	for i := 0; i < 20; i++ {
		g.drum.valueWheelPopup.StepValue(-1)
	}
	if v := nodeParams(g, n.ID).Volume; v != 0 {
		t.Fatalf("the bottom of the volume wheel is Muted (0), got %v", v)
	}
	if got := m.CenterLabel(); got != formatNodeVolumeDb(0) {
		t.Errorf("bottom label %q, want %q", got, formatNodeVolumeDb(0))
	}
}

func TestNodeDurationWheelRangeAndLive(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, nil)
	openNodeWheelByTap(t, g, tap, sbValue(fieldDur))
	m := g.drum.valueWheelPopup.model().(*NumberModel)
	if m.Min != nodeParamLimits.DurationMin || m.Max != nodeParamLimits.DurationMax {
		t.Fatalf("duration range %v..%v", m.Min, m.Max)
	}
	d0 := nodeParams(g, n.ID).Duration
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if got := nodeParams(g, n.ID).Duration; got <= d0 && !(d0 == 0 && got > 1) {
		t.Fatalf("ArrowUp must lengthen the note live: %v → %v", d0, got)
	}
}

func TestNodeLogicNAndPWheels(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, func(p *model.NodeParams) { p.LogicKind = model.LogicEveryN; p.LogicN = 2 })
	openNodeWheelByTap(t, g, tap, sbValue(fieldLogicN))
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if got := nodeParams(g, n.ID).LogicN; got != 3 {
		t.Fatalf("logic N live = %d, want 3", got)
	}
	pressKeyThroughShortcuts(g, ebiten.KeyEnter)
	advanceFrames(g, 1)

	g2, n2, tap2 := newNodeWheelGame(t, func(p *model.NodeParams) { p.LogicKind = model.LogicProbability; p.LogicP = 0.5 })
	openNodeWheelByTap(t, g2, tap2, sbValue(fieldLogicP))
	pressKeyThroughShortcuts(g2, ebiten.KeyArrowUp)
	if got := nodeParams(g2, n2.ID).LogicP; math.Abs(got-0.6) > 1e-9 {
		t.Fatalf("logic P live = %v, want 0.6 (10%% steps)", got)
	}
}

func TestNodeGrooveWheel(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, func(p *model.NodeParams) { p.GrooveKind = model.GrooveDelay; p.GroovePct = 0.25 }, secGroove)
	openNodeWheelByTap(t, g, tap, sbValue(fieldGroovePct))
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if got := nodeParams(g, n.ID).GroovePct; math.Abs(got-0.30) > 1e-9 {
		t.Fatalf("groove live = %v, want 0.30 (5%% steps)", got)
	}
}

// A tap on ANOTHER node while a node's wheel is open commits to the node the
// wheel was opened for — never to the newly selected one.
func TestNodeWheelCommitsToOriginalNodeOnRetarget(t *testing.T) {
	g, a, tap := newNodeWheelGame(t, nil)
	b := g.tryAddNode(6, 1, model.NodeTypeRegular)
	if b == nil {
		t.Fatal("second node")
	}
	vb := nodeParams(g, b.ID).Volume
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	va := nodeParams(g, a.ID).Volume
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)
	g.sidebar.Open(b) // retarget the sidebar while the wheel is open
	g.drum.CloseAllPopups()
	advanceFrames(g, 1)
	if got := nodeParams(g, a.ID).Volume; got >= va {
		t.Fatalf("node A keeps the wheel's edit: %v → %v", va, got)
	}
	if got := nodeParams(g, b.ID).Volume; got != vb {
		t.Fatalf("node B must be untouched: %v → %v", vb, got)
	}
}

// The ± steppers share the limits table: they can no longer drive a value
// outside the range the wheel and the JS bridge enforce.
func TestNodeSteppersRespectTheLimitsTable(t *testing.T) {
	g, n, _ := newNodeWheelGame(t, func(p *model.NodeParams) {
		p.Duration = 0.3
		p.LogicKind = model.LogicEveryN
		p.LogicN = 1
	})
	fire := func(id sbID, times int) {
		for i := 0; i < times; i++ {
			if b := g.sidebar.btns[id]; b != nil && b.OnClick != nil {
				b.OnClick()
				for _, fn := range g.uiQueue {
					fn()
				}
				g.uiQueue = nil
			}
		}
	}
	fire(sbMinus(fieldDur), 5)
	if got := nodeParams(g, n.ID).Duration; got < nodeParamLimits.DurationMin {
		t.Fatalf("dur- drove duration to %v (< %v)", got, nodeParamLimits.DurationMin)
	}
	fire(sbMinus(fieldLogicN), 3)
	if got := nodeParams(g, n.ID).LogicN; got < nodeParamLimits.LogicNMin {
		t.Fatalf("ln- drove N to %d (< %d)", got, nodeParamLimits.LogicNMin)
	}
	fire(sbPlus(fieldVol), 40)
	if db := 20 * math.Log10(nodeParams(g, n.ID).Volume); db > nodeParamLimits.VolumeMaxDB+0.0001 {
		t.Fatalf("vol+ drove volume to %+.2f dB (> %+.1f)", db, nodeParamLimits.VolumeMaxDB)
	}
}

// The node wheel opens over the GRID pane, whose input is not tree-routed: a
// tap on a wheel row must reach the wheel (value changes) and never fall
// through to the grid (no node added, no node selected away).
func TestNodeWheelTapInsidePopupOverTheGridReachesTheWheel(t *testing.T) {
	g, n, tap := newNodeWheelGame(t, nil)
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	w := g.drum.valueWheelPopup
	nodes0 := len(g.graph.Nodes)
	v0 := nodeParams(g, n.ID).Volume
	cx := (w.valRect.Min.X + w.valRect.Max.X) / 2
	cy := (w.valRect.Min.Y+w.valRect.Max.Y)/2 - w.tickGap() // the row ABOVE centre = louder
	g.Draw(ebiten.NewImage(1280, 720))
	tap(cx, cy, 2)
	advanceFrames(g, 2)
	if got := nodeParams(g, n.ID).Volume; got <= v0 {
		t.Fatalf("tapping the row above centre must raise the volume: %v → %v", v0, got)
	}
	if len(g.graph.Nodes) != nodes0 {
		t.Fatalf("a tap inside the wheel fell through to the grid: nodes %d → %d", nodes0, len(g.graph.Nodes))
	}
	if !w.IsOpen() {
		t.Fatal("a row tap keeps the wheel open")
	}
}

// Review I1: Esc must put the node back EXACTLY as it was — never re-derive it
// through the wheel's clamped, dB-converted model (a quiet node would come back
// Muted, a loud one capped, an exact value drifted — all outside undo).
func TestNodeWheelEscRestoresTheOpeningParamsVerbatim(t *testing.T) {
	cases := []struct {
		name string
		pill sbID
		set  func(p *model.NodeParams)
		key  ebiten.Key
	}{
		{"exact volume", sbValue(fieldVol), func(p *model.NodeParams) { p.Volume = 0.9 }, ebiten.KeyArrowUp},
		{"quiet volume", sbValue(fieldVol), func(p *model.NodeParams) { p.Volume = 1e-3 }, ebiten.KeyArrowUp},
		{"unset duration", sbValue(fieldDur), func(p *model.NodeParams) { p.Duration = 0 }, ebiten.KeyArrowUp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, n, tap := newNodeWheelGame(t, c.set)
			before := nodeParams(g, n.ID)
			undo0 := len(g.undoManager.undo)
			openNodeWheelByTap(t, g, tap, c.pill)
			pressKeyThroughShortcuts(g, c.key)
			restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
			advanceFrames(g, 1)
			restore()
			advanceFrames(g, 1)
			got := nodeParams(g, n.ID)
			if got.Volume != before.Volume || got.Duration != before.Duration || got.LogicN != before.LogicN ||
				got.LogicP != before.LogicP || got.GroovePct != before.GroovePct {
				t.Fatalf("Esc changed the node: vol %v→%v dur %v→%v", before.Volume, got.Volume, before.Duration, got.Duration)
			}
			if len(g.undoManager.undo) != undo0 {
				t.Fatalf("Esc must record no undo step")
			}
		})
	}
}

// A node quieter than the wheel's -40 dB floor (reachable with the − stepper)
// opens showing its real level, not "Muted".
func TestNodeVolumeWheelShowsAQuietNodesRealLevel(t *testing.T) {
	g, _, tap := newNodeWheelGame(t, func(p *model.NodeParams) { p.Volume = 1e-3 }) // -60 dB
	openNodeWheelByTap(t, g, tap, sbValue(fieldVol))
	if got := g.drum.valueWheelPopup.model().CenterLabel(); got != "-60 dB" {
		t.Fatalf("a -60 dB node opens as %q, want -60 dB", got)
	}
}
