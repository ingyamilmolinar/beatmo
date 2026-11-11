# Grid Guide / Crosshair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An always-on grid-guide crosshair that snaps node placement to the nearest node, the best fraction cell on the nearest edge (smallest-denominator rule), or the free cell — with beat:subdivision + fraction readout, a ghost-node preview, and a mobile drag-to-aim gesture.

**Architecture:** One pure snap engine (`guideTargetAt` in `grid_guide.go`) consumed by a new draw-only `GZGuide` layer, the existing cursor label, the desktop click-to-place path, move-mode, and a new mobile aiming gesture. Placement always commits through the existing `tryAddNode`; the existing `stitchEdgesAt` auto-split does all edge rewiring.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten stub tests (`-tags test`), DESIGN.md density tokens.

**Spec:** `docs/superpowers/specs/2026-07-13-grid-guide-crosshair-design.md`

## Global Constraints

- **NEVER run `git add` or `git commit`** — leave all changes uncommitted (standing user rule; overrides this plan template's commit steps, which are therefore omitted).
- Test command (fast path, run from repo root): `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<Name>'`
- Format with `.tools/go/bin/gofmt -w <files>` after every task.
- No new color literals — existing tokens only (`colAccentDim`, `colAccentBright`, `genColorVizGlow`, `genAlphaMedium`, `genAlphaStrong`, `genAlphaAccentOverlay`); `token_discipline_test.go` ratchets literal counts.
- Never hand-edit `*.gen.go` files — regenerate via `make gen-design-tokens` (run from repo root).
- Functional tests drive the real `Game.Update` via `SetInputForTest` + `advanceFrames` (repo discipline; see `game_marquee_test.go` for the canonical shape).
- All test files start with `//go:build test`.
- Edges are orthogonal-only (`addEdgeNoRefresh`, `game_input_editor.go` invariant) — axis-aligned math is safe everywhere.
- `abs(int) int` already exists in package `ui` (used by `touch.go`); Go 1.23 builtin `min`/`max` are available for both int and float64.

---

### Task 1: Density token + snap engine (`grid_guide.go`)

**Files:**
- Modify: `DESIGN.md` (densities YAML, 3 blocks)
- Create: `src/go/internal/ui/grid_guide.go`
- Test: `src/go/internal/ui/grid_guide_test.go`
- Regenerated: `src/go/internal/ui/design_density.gen.go` (via make target only)

**Interfaces:**
- Consumes: `g.nodes []*uiNode`, `g.edges []uiEdge`, `g.cam` (`Scale`, `OffsetX`, `OffsetY`), `g.grid.Snap/Unit`, `g.nodeAt(i, j)`, `gridTopOffset()`, `utils.GCD`, `Profile().DensityValues().GuideSnapRadius`.
- Produces (all later tasks depend on these exact names):
  - `type GuideKind int` with consts `GuideCell`, `GuideNode`, `GuideEdge`
  - `type GuideTarget struct { Kind GuideKind; I, J int; Node *uiNode; Edge *uiEdge; Num, Den int }`
  - `func (g *Game) guideTargetAt(mx, my int) GuideTarget` (screen coords in, target out)
  - `func (g *Game) guideSnapRadiusWorld() float64`
  - Test helpers `buildGuideCircuit(t, span)` and `guideScreenAt(g, wx, wy)`

- [ ] **Step 1: Add the `guideSnapRadius` density token**

In `DESIGN.md`, find the `densities:` YAML section (three blocks: `compact:`, `comfortable:`, `spacious:`). Add one line to each block, alphabetically near `grabZone`:

```yaml
  compact:
    ...
    guideSnapRadius:       16
  comfortable:
    ...
    guideSnapRadius:       20
  spacious:
    ...
    guideSnapRadius:       28
```

- [ ] **Step 2: Regenerate density tokens**

Run from repo root: `make gen-design-tokens`
Expected: `src/go/internal/ui/design_density.gen.go` gains a `GuideSnapRadius int // densities.guideSnapRadius` field with values 16/20/28. Verify: `grep GuideSnapRadius src/go/internal/ui/design_density.gen.go`

- [ ] **Step 3: Write the failing tests**

Create `src/go/internal/ui/grid_guide_test.go`:

```go
//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildGuideCircuit creates two nodes at (0,0) and (0,span) joined by one
// vertical edge, with a deterministic camera. Mirrors the canonical
// functional-test constructor (see buildGroupLoopCircuit, game_groups_test.go).
func buildGuideCircuit(t *testing.T, span int) (*Game, *uiNode, *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, span, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.cam.Scale = 2.0
	g.cam.OffsetX = 200
	g.cam.OffsetY = 60
	return g, a, b
}

// guideScreenAt converts world coords into the screen coords guideTargetAt expects.
func guideScreenAt(g *Game, wx, wy float64) (int, int) {
	return int(math.Round(g.cam.OffsetX + wx*g.cam.Scale)),
		int(math.Round(g.cam.OffsetY+wy*g.cam.Scale)) + gridTopOffset()
}

func TestGuideTargetNodeWins(t *testing.T) {
	g, a, _ := buildGuideCircuit(t, 12)
	// A point diagonally off node a, inside the snap radius but not on the edge.
	r := g.guideSnapRadiusWorld()
	sx, sy := guideScreenAt(g, 0.5*r, -0.5*r)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideNode || got.Node != a {
		t.Fatalf("want GuideNode(a), got kind=%v node=%v", got.Kind, got.Node)
	}
	if got.I != a.I || got.J != a.J {
		t.Fatalf("node target cell = (%d,%d), want (%d,%d)", got.I, got.J, a.I, a.J)
	}
}

func TestGuideSmallestDenominatorRule(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()

	// Zoomed in: radius < 1 unit → exact nearest cell. Cursor at 4.2 units
	// along the edge picks cell 4 = 1/3 of the span.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.2*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 4 || got.Num != 1 || got.Den != 3 {
		t.Fatalf("zoomed in: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,4) 1/3",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}

	// Zoomed out: radius ≈ 4 units. Cursor at 4.4 units now also has cell 6
	// (= 1/2, denominator 2) in reach — the simpler fraction wins.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (4.0 * u)
	sx, sy = guideScreenAt(g, 0.2*u, 4.4*u)
	got = g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 6 || got.Num != 1 || got.Den != 2 {
		t.Fatalf("zoomed out: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,6) 1/2",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
}

func TestGuideThirdsUnrepresentableOnSpan8(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 8)
	u := g.grid.Unit()
	// Tight radius (0.5u): only cell 3 is reachable near 8/3 ≈ 2.67 → the
	// honest fraction is 3/8 (no exact third exists on a span-8 edge).
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.5 * u)
	sx, sy := guideScreenAt(g, 0.2*u, (8.0/3.0)*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 3 || got.Num != 3 || got.Den != 8 {
		t.Fatalf("tight: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,3) 3/8",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
	// Wider radius (0.9u): cell 2 (= 1/4, denominator 4) comes into reach and
	// beats 3/8 despite being farther from the cursor.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy = guideScreenAt(g, 0.2*u, (8.0/3.0)*u)
	got = g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 2 || got.Num != 1 || got.Den != 4 {
		t.Fatalf("wide: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,2) 1/4",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
}

func TestGuideNearestEdgeWins(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	// Second parallel vertical edge at I=6.
	c := g.tryAddNode(6, 0, model.NodeTypeRegular)
	d := g.tryAddNode(6, 12, model.NodeTypeRegular)
	g.addEdge(c, d)
	// Cursor 2u from the I=0 edge, 4u from the I=6 edge; radius 2.5u.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (2.5 * u)
	sx, sy := guideScreenAt(g, 2.0*u, 5.0*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.I != 0 || got.J != 6 {
		t.Fatalf("got kind=%v cell=(%d,%d), want the nearer I=0 edge's 1/2 cell (0,6)",
			got.Kind, got.I, got.J)
	}
}

func TestGuideFreeCellFarFromCircuit(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	r := g.guideSnapRadiusWorld()
	// Well beyond the snap radius of every node and edge, whatever Step is.
	wx, wy := 3*r+40*u, 3*r+40*u
	sx, sy := guideScreenAt(g, wx, wy)
	got := g.guideTargetAt(sx, sy)
	_, _, wantI, wantJ := g.grid.Snap(wx, wy)
	if got.Kind != GuideCell || got.I != wantI || got.J != wantJ {
		t.Fatalf("got kind=%v cell=(%d,%d), want free cell (%d,%d)",
			got.Kind, got.I, got.J, wantI, wantJ)
	}
}

func TestGuideSpanOneEdgeNeverEdgeTarget(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 1)
	u := g.grid.Unit()
	for _, along := range []float64{0.2, 0.5, 0.8} {
		sx, sy := guideScreenAt(g, 0.1*u, along*u)
		if got := g.guideTargetAt(sx, sy); got.Kind == GuideEdge {
			t.Fatalf("span-1 edge produced GuideEdge at along=%v", along)
		}
	}
}

func TestGuideOccupiedCellResolvesToNode(t *testing.T) {
	// A node parked on an interior cell of a hand-built UI edge (defensive
	// case: normal placement auto-stitches, but imports/origin flows can
	// leave this shape). Built WITHOUT buildGuideCircuit: the nodes must be
	// created BEFORE any edge exists so no stitch splits the spanning edge —
	// and no g.addEdge call, so g.edges holds exactly the hand-built edge.
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 12, model.NodeTypeRegular)
	c := g.tryAddNode(0, 6, model.NodeTypeRegular) // no edges yet → no stitch
	g.edges = append(g.edges, uiEdge{A: a, B: b})  // spanning edge passing over c
	g.cam.Scale = 2.0
	g.cam.OffsetX = 200
	g.cam.OffsetY = 60
	u := g.grid.Unit()
	// Radius 1.5u; cursor 1.2u perpendicular, along-axis 5.0. The reachable
	// cells are 4..6; cell 6 (1/2, den 2) wins the smallest-denominator rule
	// but is occupied by c → GuideNode(c). c itself is hypot(1.2,1.0) ≈ 1.56u
	// from the cursor — outside the radius — so the node-search phase cannot
	// claim it first; only the occupied-cell branch can.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (1.5 * u)
	sx, sy := guideScreenAt(g, 1.2*u, 5.0*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideNode || got.Node != c {
		t.Fatalf("occupied cell: got kind=%v node=%v, want GuideNode(c)", got.Kind, got.Node)
	}
}
```

Note on the occupied-cell test: if the implemented radius/rounding makes the 1.56u-vs-1.5u margin flaky, adjust the perpendicular offset (1.2u → 1.3u) rather than the rule.

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuide'`
Expected: FAIL — `undefined: GuideNode`, `undefined: (*Game).guideTargetAt`, etc.

- [ ] **Step 5: Implement the snap engine**

Create `src/go/internal/ui/grid_guide.go`:

```go
package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// GuideKind classifies what the grid guide snapped to.
type GuideKind int

const (
	// GuideCell is a free grid cell — no circuit element within the snap radius.
	GuideCell GuideKind = iota
	// GuideNode is an existing node within the snap radius.
	GuideNode
	// GuideEdge is an interior cell on an existing edge within the snap radius.
	GuideEdge
)

// GuideTarget is the resolved commit target for the grid guide crosshair.
type GuideTarget struct {
	Kind GuideKind
	I, J int     // commit cell (always valid)
	Node *uiNode // set when Kind == GuideNode
	Edge *uiEdge // set when Kind == GuideEdge
	// Num/Den is the reduced fraction of the A→B span at the commit cell
	// (GuideEdge only): a node placed here sits Num/Den of the way from
	// Edge.A to Edge.B.
	Num, Den int
}

// guideSnapRadiusWorld converts the screen-pixel snap radius (density token)
// to world units at the current zoom. Screen-space definition makes snapping
// zoom-proportional: zoomed out the radius spans many cells (coarse musical
// snapping), zoomed in it spans less than one cell (exact placement).
func (g *Game) guideSnapRadiusWorld() float64 {
	if g.cam.Scale <= 0 {
		return 0
	}
	return float64(Profile().DensityValues().GuideSnapRadius) / g.cam.Scale
}

// guideTargetAt resolves the guide target for a screen-space cursor position:
// the nearest node within the snap radius, else the best interior cell on the
// nearest edge within the radius, else the free cell under the cursor.
func (g *Game) guideTargetAt(mx, my int) GuideTarget {
	wx := (float64(mx) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(my-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	radius := g.guideSnapRadiusWorld()

	var bestNode *uiNode
	bestNodeD := radius
	for _, n := range g.nodes {
		if d := math.Hypot(n.X-wx, n.Y-wy); d <= bestNodeD {
			bestNodeD, bestNode = d, n
		}
	}
	if bestNode != nil {
		return GuideTarget{Kind: GuideNode, I: bestNode.I, J: bestNode.J, Node: bestNode}
	}

	if t, ok := g.guideEdgeTarget(wx, wy, radius); ok {
		return t
	}

	_, _, i, j := g.grid.Snap(wx, wy)
	return GuideTarget{Kind: GuideCell, I: i, J: j}
}

// guideEdgeTarget finds the nearest edge within radius (perpendicular distance
// to the axis-aligned segment; ties broken by smaller span) and picks the
// interior cell whose reduced fraction of the span has the smallest
// denominator among the cells within radius of the cursor's along-axis
// position (nearest wins denominator ties). Edges are orthogonal-only, so all
// geometry is axis-aligned.
func (g *Game) guideEdgeTarget(wx, wy, radius float64) (GuideTarget, bool) {
	unit := g.grid.Unit()
	if unit <= 0 {
		return GuideTarget{}, false
	}
	var best *uiEdge
	bestDist := radius
	bestSpan := 0
	for i := range g.edges {
		e := &g.edges[i]
		a, b := e.A, e.B
		var dist float64
		var span int
		switch {
		case a.I == b.I: // vertical
			span = abs(b.J - a.J)
			along := min(max(wy, min(a.Y, b.Y)), max(a.Y, b.Y))
			dist = math.Hypot(wx-a.X, wy-along)
		case a.J == b.J: // horizontal
			span = abs(b.I - a.I)
			along := min(max(wx, min(a.X, b.X)), max(a.X, b.X))
			dist = math.Hypot(wy-a.Y, wx-along)
		default:
			continue
		}
		if span < 2 { // adjacent nodes: no interior cells
			continue
		}
		if dist < bestDist || (dist == bestDist && (best == nil || span < bestSpan)) {
			bestDist, best, bestSpan = dist, e, span
		}
	}
	if best == nil {
		return GuideTarget{}, false
	}

	a, b := best.A, best.B
	vertical := a.I == b.I
	var aU, bU int
	var alongW float64
	if vertical {
		aU, bU, alongW = a.J, b.J, wy
	} else {
		aU, bU, alongW = a.I, b.I, wx
	}
	span := abs(bU - aU)
	dir := 1
	if bU < aU {
		dir = -1
	}
	loU, hiU := min(aU, bU)+1, max(aU, bU)-1
	alongU := alongW / unit
	radiusU := radius / unit
	kMin := max(int(math.Ceil(alongU-radiusU)), loU)
	kMax := min(int(math.Floor(alongU+radiusU)), hiU)
	if kMin > kMax {
		// No interior cell within radius along the axis: nearest interior cell.
		k := min(max(int(math.Round(alongU)), loU), hiU)
		kMin, kMax = k, k
	}
	bestK := kMin
	bestDen := span + 1
	bestOff := math.MaxFloat64
	for k := kMin; k <= kMax; k++ {
		off := (k - aU) * dir // 1..span-1 measured from A
		den := span / utils.GCD(off, span)
		d := math.Abs(float64(k) - alongU)
		if den < bestDen || (den == bestDen && d < bestOff) {
			bestDen, bestK, bestOff = den, k, d
		}
	}
	off := (bestK - aU) * dir
	gcd := utils.GCD(off, span)
	ci, cj := a.I, bestK
	if !vertical {
		ci, cj = bestK, a.J
	}
	if n := g.nodeAt(ci, cj); n != nil {
		// Cell already occupied (crossing circuit / imported shape).
		return GuideTarget{Kind: GuideNode, I: n.I, J: n.J, Node: n}, true
	}
	return GuideTarget{Kind: GuideEdge, I: ci, J: cj, Edge: best, Num: off / gcd, Den: span / gcd}, true
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuide'`
Expected: PASS (all 7).

- [ ] **Step 7: gofmt + design guard tests**

Run: `.tools/go/bin/gofmt -w src/go/internal/ui/grid_guide.go src/go/internal/ui/grid_guide_test.go`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS (density tokens are not in the drift want-map; nothing else changed).

---

### Task 2: GZGuide render layer + cursor-label fraction chip

**Files:**
- Create: `src/go/internal/ui/grid_guide_draw.go`
- Modify: `src/go/internal/ui/grid_tree.go` (GZ consts), `src/go/internal/ui/grid_pane_layers.go` (registration), `src/go/internal/ui/grid_pane_draw.go:1281-1309` (`drawGridCursorLabel`)
- Test: `src/go/internal/ui/grid_guide_draw_test.go`, modify `src/go/internal/ui/grid_layer_zorder_test.go`

**Interfaces:**
- Consumes: `guideTargetAt`, `g.gridDrawCtx` (`unitPx`, `offX`, `offY`, `gridRect`), `cursorPosition()`, `DrawLineCam`, `drawRect`, `g.grid.NodeRadius`, tokens.
- Produces: `GZGuide = 35` const; layer id `"grid-guide"`; `func (g *Game) guideLayerVisible() bool`; `func (g *Game) drawGridGuide(dst *ebiten.Image)`. Task 4 later extends `guideLayerVisible` and the label gate with the aiming state.

- [ ] **Step 1: Write the failing tests**

Modify `grid_layer_zorder_test.go`: insert `{"grid-guide", GZGuide},` into the `want` list between `{"grid-pulses", GZPulses}` and `{"grid-coord-badge", GZCoordBadge}`.

Create `src/go/internal/ui/grid_guide_draw_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// setCursorForGuideTest pins the package cursor at (x,y) with no buttons held.
func setCursorForGuideTest(t *testing.T, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
}

func TestGuideCursorLabelShowsFraction(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.1*u)
	setCursorForGuideTest(t, sx, sy)
	advanceFrames(g, 2)
	img := ebiten.NewImage(1280, 720)
	g.Draw(img)
	if !strings.Contains(g.cursorLabel, "1/3") {
		t.Fatalf("cursor label %q must contain the 1/3 span fraction", g.cursorLabel)
	}
}

func TestGuideGhostDrawnAtCommitCell(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.1*u)
	setCursorForGuideTest(t, sx, sy)
	advanceFrames(g, 2)

	// Intercept drawRect (package function variable) to observe the ghost.
	var rects []image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, r)
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	img := ebiten.NewImage(1280, 720)
	g.Draw(img)

	// Expected ghost center: screen position of cell (0,4). Filter to
	// ghost-sized rects — large rects (backgrounds, panels) would contain the
	// point trivially and mask a missing ghost.
	cx := int(g.cam.OffsetX)
	cy := int(g.cam.OffsetY+4*u*g.cam.Scale) + gridTopOffset()
	found := false
	for _, r := range rects {
		if r.Dx() <= 60 && r.Dy() <= 60 && image.Pt(cx, cy).In(r.Inset(-2)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no drawRect covering ghost center (%d,%d); %d rects seen", cx, cy, len(rects))
	}
}

func TestGuideLayerHiddenOutsideGridPane(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	setCursorForGuideTest(t, 10, g.winH-10) // deep in the drum pane
	advanceFrames(g, 2)
	if g.guideLayerVisible() {
		t.Fatalf("guide must be hidden while the cursor is outside the grid pane")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridLayerZOrder|TestGuideCursorLabel|TestGuideGhost|TestGuideLayerHidden'`
Expected: FAIL — `undefined: GZGuide`, `undefined: (*Game).guideLayerVisible`, z-order count mismatch.

- [ ] **Step 3: Implement the layer**

In `grid_tree.go`, add to the const block after `GZPulses = 30`:

```go
	GZGuide          = 35
```

In `grid_pane_layers.go` `registerGridTree`, insert after the `grid-pulses` line:

```go
	g.gridTree.RegisterLayer(gridLayer{id: "grid-guide", z: GZGuide, clip: true, vis: g.guideLayerVisible, draw: g.drawGridGuide})
```

Create `src/go/internal/ui/grid_guide_draw.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// guideLayerVisible gates the grid-guide crosshair layer. Desktop: whenever
// the pointer is inside the grid pane (same gate as the cursor label). Touch:
// only while a finger is down — otherwise the crosshair would stick at the
// last lift position.
func (g *Game) guideLayerVisible() bool {
	if g.modalOverlayActive() || g.landscapeUnsupported() {
		return false
	}
	mx, my := cursorPosition()
	if my < gridTopOffset() || !g.split.InGridPane(mx, my) {
		return false
	}
	if touchOverrideActive {
		return true
	}
	return Profile().ShowCursorLabel
}

// drawGridGuide renders the placement guide: a screen-constant crosshair
// through the snapped target, fraction ticks + a brightened line on the
// active edge, and a ghost-node preview at the commit cell. All chrome except
// the ghost is screen-constant so nothing grows when zooming in; the ghost
// previews a real node, so it scales with zoom like one (see NodeRadius).
func (g *Game) drawGridGuide(dst *ebiten.Image) {
	if !g.guideLayerVisible() {
		return
	}
	ctx := &g.gridDrawCtx
	mx, my := cursorPosition()
	t := g.guideTargetAt(mx, my)
	unitPx := ctx.unitPx
	top := float64(gridTopOffset())
	sx := ctx.offX + unitPx*float64(t.I)
	sy := ctx.offY + unitPx*float64(t.J) + top
	gridRect := ctx.gridRect
	var idm ebiten.GeoM
	cross := WithAlpha(colAccentDim, genAlphaMedium)
	DrawLineCam(dst, sx, float64(gridRect.Min.Y), sx, float64(gridRect.Max.Y), &idm, cross, 1)
	DrawLineCam(dst, float64(gridRect.Min.X), sy, float64(gridRect.Max.X), sy, &idm, cross, 1)

	if t.Kind == GuideEdge && t.Edge != nil {
		g.drawGuideEdgeTicks(dst, t, &idm)
	}

	// Ghost node preview at the commit cell. Move mode draws its own ghost
	// (drawGridMoveMode); a node target needs no preview — the node exists.
	if t.Kind != GuideNode && !(g.moveMode && g.movingNode != nil) {
		gr := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
		ghostRect := image.Rect(int(sx-gr), int(sy-gr), int(sx+gr), int(sy+gr))
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaAccentOverlay), true)
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaStrong), false)
	}
}

// drawGuideEdgeTicks brightens the active edge and marks its simple-fraction
// positions (reduced denominator 2, 3, or 4) with small screen-constant ticks.
func (g *Game) drawGuideEdgeTicks(dst *ebiten.Image, t GuideTarget, idm *ebiten.GeoM) {
	ctx := &g.gridDrawCtx
	unitPx := ctx.unitPx
	top := float64(gridTopOffset())
	a, b := t.Edge.A, t.Edge.B
	ax := ctx.offX + unitPx*float64(a.I)
	ay := ctx.offY + unitPx*float64(a.J) + top
	bx := ctx.offX + unitPx*float64(b.I)
	by := ctx.offY + unitPx*float64(b.J) + top
	bright := WithAlpha(colAccentBright, genAlphaStrong)
	DrawLineCam(dst, ax, ay, bx, by, idm, bright, 1)

	vertical := a.I == b.I
	span := abs(b.I - a.I)
	if vertical {
		span = abs(b.J - a.J)
	}
	const tickPx = 4.0
	for _, den := range [3]int{2, 3, 4} {
		if span%den != 0 {
			continue
		}
		for num := 1; num < den; num++ {
			if utils.GCD(num, den) != 1 {
				continue // 2/4 etc. already drawn at a smaller denominator
			}
			if vertical {
				fy := ay + unitPx*float64((b.J-a.J)*num/den)
				DrawLineCam(dst, ax-tickPx, fy, ax+tickPx, fy, idm, bright, 1)
			} else {
				fx := ax + unitPx*float64((b.I-a.I)*num/den)
				DrawLineCam(dst, fx, ay-tickPx, fx, ay+tickPx, idm, bright, 1)
			}
		}
	}
}
```

- [ ] **Step 4: Rewrite `drawGridCursorLabel` to read the guide target**

In `grid_pane_draw.go`, replace the body of `drawGridCursorLabel` (lines 1281-1309) with:

```go
func (g *Game) drawGridCursorLabel(dst *ebiten.Image) {
	// cursor coordinate label (hidden on mobile)
	mx, my := cursorPosition()
	if Profile().ShowCursorLabel && g.split.InGridPane(mx, my) {
		t := g.guideTargetAt(mx, my)
		bx, nx, dx := g.grid.BeatSubdivision(t.I)
		by, ny, dy := g.grid.BeatSubdivision(t.J)
		xs := "0"
		ys := "0"
		if nx != 0 {
			xs = fmt.Sprintf("%d/%d", nx, dx)
		}
		if ny != 0 {
			ys = fmt.Sprintf("%d/%d", ny, dy)
		}
		g.cursorLabel = fmt.Sprintf("(%d:%s, %d:%s)", bx, xs, by, ys)
		if t.Kind == GuideEdge {
			// Fraction of the span between the edge's two endpoint nodes.
			g.cursorLabel += fmt.Sprintf(" %d/%d", t.Num, t.Den)
		}
		DrawTextAt(dst, g.cursorLabel, mx+8, my+16)
	} else {
		g.cursorLabel = ""
	}
}
```

(The `ctx`/`unitPx`/`offX`/`offY` locals and the manual world conversion go away — `guideTargetAt` owns the transform. The label now reports the SNAPPED cell, matching the crosshair.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridLayerZOrder|TestGuide'`
Expected: PASS.

- [ ] **Step 6: gofmt + neighborhood regression check**

Run: `.tools/go/bin/gofmt -w src/go/internal/ui/grid_guide_draw.go src/go/internal/ui/grid_pane_draw.go src/go/internal/ui/grid_tree.go src/go/internal/ui/grid_pane_layers.go src/go/internal/ui/grid_layer_zorder_test.go src/go/internal/ui/grid_guide_draw_test.go`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGrid|TestCursor|Discipline'`
Expected: PASS (notably `grid_render_pipeline_discipline_test.go` and `grid_draw_dispatch_discipline_test.go` still green — the new layer follows the drawGrid* pattern).

---

### Task 3: Desktop placement + move-mode integration

**Files:**
- Modify: `src/go/internal/ui/game_input_editor.go` (move-mode Snap at lines 86-88; empty-intersection release branch at lines 280-296)
- Modify: `src/go/internal/ui/game_input_touch.go` (move-mode Snap at lines 58-60)
- Modify: `src/go/internal/ui/grid_pane_draw.go` (`drawGridMoveMode` ghost, lines 1242-1244)
- Test: `src/go/internal/ui/grid_guide_input_test.go`

**Interfaces:**
- Consumes: `guideTargetAt`, `GuideNode`/`GuideEdge`/`GuideCell`, `tryAddNode`, `g.performUndo()`.
- Produces: no new API — behavior change only (click resolves through the guide).

- [ ] **Step 1: Write the failing tests**

Create `src/go/internal/ui/grid_guide_input_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// clickAt simulates press+release at a fixed screen position through the real
// Update loop (mirrors shiftDrag in game_marquee_test.go, without Shift).
func clickAt(g *Game, x, y int) {
	down := false
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	down = true
	advanceFrames(g, 2)
	down = false
	advanceFrames(g, 2)
}

func TestGuideClickInsertsNodeAtThirdAndStitches(t *testing.T) {
	g, a, b := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.3*u, 4.2*u)
	clickAt(g, sx, sy)

	n := g.nodeAt(0, 4)
	if n == nil {
		t.Fatalf("expected a node at (0,4) — 1/3 of the a→b span")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; ok {
		t.Fatalf("a→b must be split by the auto-stitch")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, n.ID}]; !ok {
		t.Fatalf("missing stitched edge a→new")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{n.ID, b.ID}]; !ok {
		t.Fatalf("missing stitched edge new→b")
	}

	// One undo removes the node AND restores the original edge (one group).
	g.performUndo()
	if g.nodeAt(0, 4) != nil {
		t.Fatalf("undo must remove the inserted node")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("undo must restore the original a→b edge")
	}
}

func TestGuideClickNearNodeSelectsInsteadOfCreating(t *testing.T) {
	g, a, _ := buildGuideCircuit(t, 12)
	r := g.guideSnapRadiusWorld()
	// Diagonally off node a, inside snap radius, clear of the edge line.
	sx, sy := guideScreenAt(g, 0.5*r, -0.5*r)
	before := len(g.nodes)
	clickAt(g, sx, sy)
	if len(g.nodes) != before {
		t.Fatalf("click near a node must select, not create (nodes %d→%d)", before, len(g.nodes))
	}
	if g.sel != a {
		t.Fatalf("node a must be selected")
	}
	if !g.sidebar.IsOpen() {
		t.Fatalf("selecting a node by click opens the sidebar")
	}
}

func TestGuideMoveModeSnapsGhostTarget(t *testing.T) {
	g, _, b := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	// Enter move mode for b, then tap-place at a free cell IN THE SAME COLUMN
	// (destination (0,20) keeps the a→b edge orthogonal, so moveNodeEdgeLoss
	// is 0 and the move commits instead of opening the confirm dialog). The
	// commit must go through the guide; the edge-fraction path is covered by
	// the shared engine tests.
	g.moveMode = true
	g.movingNode = b
	sx, sy := guideScreenAt(g, 0.2*u, 20.1*u)
	g.handleTapInGrid(sx, sy)
	if b.I != 0 || b.J != 20 {
		t.Fatalf("move-mode placement = (%d,%d), want guide-snapped (0,20)", b.I, b.J)
	}
	if g.moveConfirm {
		t.Fatalf("orthogonality-preserving move must not open the confirm dialog")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuideClick|TestGuideMoveMode'`
Expected: `TestGuideClickInsertsNodeAtThirdAndStitches` FAILs (node lands at the raw cursor cell, e.g. (0,4) only if the cursor happened to round there — with the 4.2u cursor the raw snap gives (0,4) too, so the decisive failure is `TestGuideClickNearNodeSelectsInsteadOfCreating`: today it CREATES a node next to a). Expect at least that test failing; move-mode test fails on the guide-snapped cell if raw rounding differs (8.2→8 rounds the same, so it may pass pre-change — keep it as a regression pin).

- [ ] **Step 3: Route desktop release + move mode through the guide**

In `game_input_editor.go`, move-mode placement (current lines 86-88) — replace:

```go
			wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
			wy := (float64(y-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
			_, _, ni, nj := g.grid.Snap(wx, wy)
```

with:

```go
			gt := g.guideTargetAt(x, y)
			ni, nj := gt.I, gt.J
```

In the same file, the empty-intersection release branch (current lines 281-296) — replace:

```go
			} else {
				// Empty intersection → add/select regular node and close menu
				g.logger.Tracef("[input/node] add/select grid=(%d,%d)", g.clickI, g.clickJ)
				n := g.tryAddNode(g.clickI, g.clickJ, model.NodeTypeRegular)
```

with:

```go
			} else if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				// Guide snapped to an existing node: select it (same as a
				// direct node click) instead of creating a neighbor.
				if g.sel != nil {
					g.sel.Selected = false
				}
				g.sel = gt.Node
				gt.Node.Selected = true
				g.sidebar.Open(gt.Node)
				g.groupMenu.Close()
				g.computeSelNeighbors()
				g.coordBadgeNode = gt.Node
				g.coordBadgeFrame = g.frame
			} else {
				// Empty intersection → add/select at the guide's commit cell
				// (edge-fraction cell or free cell); stitchEdgesAt splits any
				// edge passing through it.
				g.logger.Tracef("[input/node] add/select grid=(%d,%d)", gt.I, gt.J)
				n := g.tryAddNode(gt.I, gt.J, model.NodeTypeRegular)
```

(The rest of the original `else` body — the select/deselect block and `g.sidebar.Close()` / `g.groupMenu.Close()` — stays unchanged. Note the `gt :=` short-var lives in the `else if` header so both branches see it.)

In `game_input_touch.go`, move-mode tap placement (current lines 58-60) — same replacement as the editor's move-mode block:

```go
		gt := g.guideTargetAt(x, y)
		ni, nj := gt.I, gt.J
```

In `grid_pane_draw.go` `drawGridMoveMode`, the ghost (current lines 1242-1244) — replace:

```go
			gwx := (float64(cmx) - g.cam.OffsetX) / g.cam.Scale
			gwy := (float64(cmy-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
			_, _, gi, gj := g.grid.Snap(gwx, gwy)
```

with:

```go
			gt := g.guideTargetAt(cmx, cmy)
			gi, gj := gt.I, gt.J
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuide'`
Expected: PASS (all Task 1-3 guide tests).

- [ ] **Step 5: gofmt + editor-neighborhood regression sweep**

Run: `.tools/go/bin/gofmt -w src/go/internal/ui/game_input_editor.go src/go/internal/ui/game_input_touch.go src/go/internal/ui/grid_pane_draw.go src/go/internal/ui/grid_guide_input_test.go`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMarquee|TestMove|TestNode|TestUndo|TestGroup|TestTap'`
Expected: PASS. If any node-placement test fails because a click near an existing node/edge now snaps (changed behavior by design), inspect: the test's intent decides — tests that assert raw-cell placement AT a position within the snap radius of a circuit element must be updated to click farther away (≥ the snap radius) with a one-line comment; report any such edit in the task summary.

---

### Task 4: Mobile aiming gesture

**Files:**
- Modify: `src/go/internal/ui/game_input_gesture_state.go` (Game fields), `src/go/internal/ui/grid_guide.go` (aim logic + commit), `src/go/internal/ui/game_update.go` (call site ~line 291; `panOK` ~line 604), `src/go/internal/ui/game_input_touch.go` (`handleTapInGrid` empty branch), `src/go/internal/ui/grid_guide_draw.go` (visibility + label offset)
- Test: append to `src/go/internal/ui/grid_guide_input_test.go`

**Interfaces:**
- Consumes: `guideTargetAt`, `commitGuideTarget` (new), `touchOverrideActive`, `tapMaxMovePx` (touch.go), `globalTouchState.ActiveTouchCount()`, `g.blocksAt`, `g.split.InGridPane`, `gridTopOffset()`.
- Produces: `g.guideAim guideAimState` (fields `active`, `prevLeft`, `startX`, `startY`, `moved`); `func (g *Game) updateGuideAim(mx, my int, left bool)`; `func (g *Game) commitGuideTarget(x, y int)`; `func SetGuideAimTouchForTest(v bool) func()`.

- [ ] **Step 1: Write the failing tests**

Append to `grid_guide_input_test.go`:

```go
// dragTouch simulates a touch press at (x1,y1), drag to (x2,y2), release,
// with the guide-aim touch seam armed (SetInputForTest forces the real touch
// override off inside updateTouchOverride, so the seam stands in for it).
func dragTouch(g *Game, x1, y1, x2, y2 int) {
	restoreTouch := SetGuideAimTouchForTest(true)
	defer restoreTouch()
	pos := [2]int{x1, y1}
	down := false
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	down = true
	advanceFrames(g, 2) // press edge: arm if near a circuit element
	pos = [2]int{x2, y2}
	advanceFrames(g, 3) // drag
	down = false
	advanceFrames(g, 2) // release edge: commit if moved
}

func TestGuideAimDragPlacesNodeAndSuppressesPan(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	x1, y1 := guideScreenAt(g, 0.5*u, 7.0*u) // touch-down near the edge
	x2, y2 := guideScreenAt(g, 0.3*u, 4.1*u) // aim at the 1/3 cell
	offXBefore, offYBefore := g.cam.OffsetX, g.cam.OffsetY
	dragTouch(g, x1, y1, x2, y2)
	if g.cam.OffsetX != offXBefore || g.cam.OffsetY != offYBefore {
		t.Fatalf("aiming drag must not pan the camera")
	}
	if g.nodeAt(0, 4) == nil {
		t.Fatalf("release must place the node at the aimed 1/3 cell (0,4)")
	}
}

func TestGuideDragFromEmptySpaceStillPans(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	r := g.guideSnapRadiusWorld()
	// Horizontally clear of the circuit (perp > 3r from the I=0 edge) but at
	// small y so the touch stays inside the grid pane regardless of Step.
	x1, y1 := guideScreenAt(g, 30*u+3*r, 2.0*u)
	x2, y2 := x1-60, y1+20
	before := len(g.nodes)
	offXBefore := g.cam.OffsetX
	dragTouch(g, x1, y1, x2, y2)
	if g.cam.OffsetX == offXBefore {
		t.Fatalf("drag from empty space must still pan the camera")
	}
	if len(g.nodes) != before {
		t.Fatalf("pan must not create nodes (%d→%d)", before, len(g.nodes))
	}
}

func TestGuideTapInGridSnapsThroughGuide(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.3*u, 4.2*u)
	g.handleTapInGrid(sx, sy) // the GestureTap direct handler
	if g.nodeAt(0, 4) == nil {
		t.Fatalf("grid tap must resolve through the guide to (0,4)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuideAim|TestGuideDrag|TestGuideTap'`
Expected: FAIL — `undefined: SetGuideAimTouchForTest`; the tap test fails only if raw rounding differs (0.3u→0), so its decisive assertion is the (0,4) cell via the 4.2u cursor: raw Snap gives (0,4) here too — strengthen nothing; it pins the guide path once wired.

- [ ] **Step 3: Implement the aiming gesture**

In `game_input_gesture_state.go`, next to `marquee marqueeDrag`, add:

```go
	guideAim guideAimState
```

In `grid_guide.go`, append:

```go
// guideAimState tracks the mobile aiming gesture: a touch that starts within
// the guide snap radius of a node/edge suppresses camera pan for that touch
// and commits the guide target on release (drag-to-aim). Desktop mouse input
// never arms it (touch-originated input only).
type guideAimState struct {
	active   bool
	prevLeft bool
	startX   int
	startY   int
	moved    bool
}

// guideAimTouchForTest stands in for touchOverrideActive in functional tests:
// SetInputForTest forces the real touch override off (updateTouchOverride),
// so tests arm this seam instead.
var guideAimTouchForTest bool

// SetGuideAimTouchForTest simulates touch-originated input for the aiming
// gesture. Returns a restore func.
func SetGuideAimTouchForTest(v bool) func() {
	old := guideAimTouchForTest
	guideAimTouchForTest = v
	return func() { guideAimTouchForTest = old }
}

// guideAimTouchActive reports whether this frame's pointer input comes from a
// touch (the aiming gesture is touch-only; desktop pans on plain drags).
func guideAimTouchActive() bool { return touchOverrideActive || guideAimTouchForTest }

// updateGuideAim runs once per frame from Game.Update, before pan gating.
// Press edge near a circuit element arms aiming; movement past the tap slop
// marks it a drag; release commits the guide target (a short tap instead
// resolves through GestureTap → handleTapInGrid on the same frame).
func (g *Game) updateGuideAim(mx, my int, left bool) {
	prev := g.guideAim.prevLeft
	g.guideAim.prevLeft = left
	if !guideAimTouchActive() {
		g.guideAim.active = false
		return
	}
	if globalTouchState.ActiveTouchCount() >= 2 || g.modalOverlayActive() ||
		g.landscapeUnsupported() || g.moveMode || g.connectMode ||
		g.longPressPopup || g.sidebar.IsOpen() {
		g.guideAim.active = false
		return
	}
	if left && !prev {
		if my >= gridTopOffset() && g.split.InGridPane(mx, my) && !g.blocksAt(mx, my) {
			if t := g.guideTargetAt(mx, my); t.Kind != GuideCell {
				g.guideAim = guideAimState{active: true, prevLeft: true, startX: mx, startY: my}
			}
		}
		return
	}
	if !g.guideAim.active {
		return
	}
	if left {
		if abs(mx-g.guideAim.startX) >= tapMaxMovePx || abs(my-g.guideAim.startY) >= tapMaxMovePx {
			g.guideAim.moved = true
		}
		return
	}
	// Release edge.
	moved := g.guideAim.moved
	g.guideAim.active = false
	g.guideAim.moved = false
	if !moved {
		return // short tap: GestureTap → handleTapInGrid owns it
	}
	if my < gridTopOffset() || !g.split.InGridPane(mx, my) {
		return // released outside the grid pane: cancel
	}
	g.commitGuideTarget(mx, my)
	// Don't let handleEditor's pendingClick release add a second node this frame.
	g.pendingClick = false
}

// commitGuideTarget places/selects at the guide target — shared by the aiming
// gesture's release and handleTapInGrid's empty-space branch.
func (g *Game) commitGuideTarget(x, y int) {
	t := g.guideTargetAt(x, y)
	if t.Kind == GuideNode {
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = t.Node
		t.Node.Selected = true
		g.sidebar.Open(t.Node)
		g.computeSelNeighbors()
		g.coordBadgeNode = t.Node
		g.coordBadgeFrame = g.frame
		return
	}
	n := g.tryAddNode(t.I, t.J, model.NodeTypeRegular)
	if n != nil {
		if g.sel != nil && g.sel != n {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
	}
	// Don't open the sidebar for new nodes (consistent with mouse behavior).
	g.sidebar.Close()
}
```

`commitGuideTarget` needs `model` — add `"github.com/ingyamilmolinar/beatmo/core/model"` to `grid_guide.go` imports and use `model.NodeTypeRegular`.

In `game_update.go`, immediately after `g.handleGlobalShortcuts()` (~line 291), add:

```go
	// Mobile aiming gesture: a touch starting near a node/edge aims the grid
	// guide instead of panning; release commits. Must run before panOK below.
	g.updateGuideAim(mx, my, left)
```

In `game_update.go` `panOK` (~line 604), append the new condition:

```go
	panOK := !g.linkDrag.active && !g.marquee.active && !g.guideAim.active && !g.split.dragging && ...
```

(keep the rest of the expression exactly as-is).

In `game_input_touch.go` `handleTapInGrid`, replace the tail (current lines 165-179, from the `// Tap on empty space creates node` comment through `g.sidebar.Close()`) with:

```go
	// Tap on empty space: resolve through the grid guide (nearest node/edge
	// cell within the snap radius; free cell otherwise).
	g.commitGuideTarget(x, y)
```

In `grid_guide_draw.go`:
- `guideLayerVisible`: change the touch branch to `if touchOverrideActive || g.guideAim.active {` (aim seam keeps the guide visible in tests and across the one-frame release hold).
- `drawGridCursorLabel` (grid_pane_draw.go): change the gate to `if (Profile().ShowCursorLabel || g.guideAim.active) && g.split.InGridPane(mx, my) {` and the draw call to:

```go
		ly := my + 16
		if guideAimTouchActive() {
			ly = my - 28 // above the finger so the hand doesn't cover it
		}
		DrawTextAt(dst, g.cursorLabel, mx+8, ly)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGuide'`
Expected: PASS (all guide tests, Tasks 1-4).

- [ ] **Step 5: gofmt + touch/pan regression sweep**

Run: `.tools/go/bin/gofmt -w src/go/internal/ui/grid_guide.go src/go/internal/ui/grid_guide_draw.go src/go/internal/ui/game_update.go src/go/internal/ui/game_input_touch.go src/go/internal/ui/game_input_gesture_state.go src/go/internal/ui/grid_guide_input_test.go src/go/internal/ui/grid_pane_draw.go`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTouch|TestPan|TestPads|TestTap|TestPinch|TestMarquee|TestLongPress'`
Expected: PASS.

---

### Task 5: Full-suite verification

**Files:** none new — verification only.

- [ ] **Step 1: Full fast Go suite for the touched packages**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/timeline ./core/...`
Expected: PASS. (`internal/ui` runs ~10min cumulatively; do not pass `-v`.)

- [ ] **Step 2: gofmt sweep**

Run: `.tools/go/bin/gofmt -l src/go/internal/ui/`
Expected: no output.

- [ ] **Step 3: Spec-coverage checklist**

Confirm each against the code (no action if green):
- Crosshair always-on desktop, touch-gated mobile (`guideLayerVisible`)
- Smallest-denominator rule + zoom-proportional radius (`guideEdgeTarget` + engine tests)
- Ghost node world-scaled, chrome screen-constant (`drawGridGuide`)
- Fraction readout in cursor label, offset above finger on touch
- Desktop click + move-mode + mobile tap + mobile drag-aim all resolve through `guideTargetAt`
- Auto-stitch split + single-undo-group verified by `TestGuideClickInsertsNodeAtThirdAndStitches`
- `grid_layer_zorder_test.go` updated; discipline tests green
- Report results; leave everything uncommitted.
