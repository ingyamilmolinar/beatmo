//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Task 4: edge-only move placement, immediate commit, target-edge stitch, and
// retirement of the move-confirm dialog. These tests drive the real
// handleTapInGrid entry point (touch path) — the same one the shared
// handleArmedModeTap helper serves — never poking gridModeManager/uiNode
// fields the input handlers wouldn't touch themselves.
//
// Shared layout (mirrors the task brief):
//
//	A(0,0) ── edge ── B(0,20)   (vertical edge, interior cells (0,1)..(0,19))
//	M(10,0)                     (the node being moved; no edges)
//
// A free empty cell (50,50) and node B itself stand in for the brief's "free
// cell C" / "occupied endpoint O" targets.

// shrinkGuideSnapRadius sets the camera scale so guideSnapRadiusWorld is a
// small fraction of one grid unit. A tap exactly on an integer grid
// coordinate then resolves deterministically — exact node/edge coincidence
// only, never an accidental proximity snap to a nearby integer coordinate —
// so the small integer coordinates in the layout above behave as the brief's
// diagram implies regardless of the default camera scale/grid subdivision.
func shrinkGuideSnapRadius(g *Game) {
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.1 * u)
}

// buildMoveModeLayout builds the shared A-B edge + free-standing M circuit
// described above, with the guide-snap radius shrunk for deterministic
// tap resolution.
func buildMoveModeLayout(t *testing.T) (g *Game, a, b, m *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(0, 20, model.NodeTypeRegular)
	g.addEdge(a, b)
	m = g.tryAddNode(10, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	shrinkGuideSnapRadius(g)
	return g, a, b, m
}

func TestMoveMode_TapOnEdgePointCommitsImmediately(t *testing.T) {
	g, a, b, m := buildMoveModeLayout(t)
	if !hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("setup: expected A-B edge before move")
	}

	g.modes.Enter(GridModeMove, m)
	tx, ty := screenPosForGridIJ(g, 0, 10)
	g.handleTapInGrid(tx, ty)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected move committed (mode=None), got %v", g.modes.Active())
	}
	moved := g.nodeAt(0, 10)
	if moved == nil || moved.ID != m.ID {
		t.Fatalf("expected M at (0,10), got node=%v", moved)
	}
	if g.nodeAt(10, 0) != nil {
		t.Fatal("expected M's old cell (10,0) to be empty after the move")
	}
	if hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("expected the A-B edge to be gone (stitched away)")
	}
	if !hasEdgeBetween(g, a.ID, m.ID) {
		t.Fatal("expected a stitched A-M edge")
	}
	if !hasEdgeBetween(g, m.ID, b.ID) {
		t.Fatal("expected a stitched M-B edge")
	}
}

func TestMoveMode_TapEmptyCellCancels(t *testing.T) {
	g, _, _, m := buildMoveModeLayout(t)
	nodesBefore := len(g.nodes)
	edgesBefore := len(g.edges)

	g.modes.Enter(GridModeMove, m)
	tx, ty := screenPosForGridIJ(g, 50, 50) // free cell, far from anything
	g.handleTapInGrid(tx, ty)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled, got %v", g.modes.Active())
	}
	if m.I != 10 || m.J != 0 {
		t.Fatalf("expected M to stay at (10,0), got (%d,%d)", m.I, m.J)
	}
	if len(g.nodes) != nodesBefore {
		t.Fatalf("node count changed: got %d want %d", len(g.nodes), nodesBefore)
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("edge count changed: got %d want %d", len(g.edges), edgesBefore)
	}
}

func TestMoveMode_TapOccupiedNodeCancels(t *testing.T) {
	g, _, b, m := buildMoveModeLayout(t)

	g.modes.Enter(GridModeMove, m)
	bx, by := nodeScreenCenter(g, b)
	g.handleTapInGrid(bx, by)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled, got %v", g.modes.Active())
	}
	if m.I != 10 || m.J != 0 {
		t.Fatalf("expected M to stay put, got (%d,%d)", m.I, m.J)
	}
}

// TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently: under extraction
// semantics, this topology's own-edge pair (M3->M2 horizontal in, M2->D
// vertical out) is a corner — M3 and D are NOT mutually orthogonal — so
// moveHealPlan classifies M2 as healUnhealable, not healPassThrough. A
// direct moveNode call still breaks silently (both of M2's own edges are
// extracted, no heal edge, no panic); the interactive confirm dialog for
// this case is Task 3's job (noted below), so this test only exercises the
// classification + the direct-call silent-break path, plus the destination
// splice into the A-B edge which is unaffected by M2's own-edge fate.
func TestMoveMode_MoveWithOwnEdgesDropsNonOrthogonalSilently(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 20, model.NodeTypeRegular)
	g.addEdge(a, b)

	m2 := g.tryAddNode(0, 30, model.NodeTypeRegular)
	d := g.tryAddNode(0, 40, model.NodeTypeRegular)
	g.addEdge(m2, d) // vertical: same I as m2's destination on the A-B edge

	m3 := g.tryAddNode(10, 30, model.NodeTypeRegular)
	g.addEdge(m3, m2) // horizontal: M3-M2-D is a corner, not a straight pass-through

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)

	if plan := g.moveHealPlan(m2); plan.class != healUnhealable {
		t.Fatalf("setup: expected M2's corner topology to classify healUnhealable, got %v", plan.class)
	}

	// Direct moveNode call (bypasses the GuideEdge-gated interactive tap
	// path; Task 3 wires the interactive confirm dialog for this case).
	if ok := g.moveNode(m2, 0, 10); !ok { // interior point of the A-B edge
		t.Fatal("moveNode returned false")
	}

	moved := g.nodeAt(0, 10)
	if moved == nil || moved.ID != m2.ID {
		t.Fatalf("expected M2 at (0,10), got node=%v", moved)
	}
	if hasEdgeBetween(g, m2.ID, d.ID) {
		t.Fatal("expected the M2-D edge extracted (all of M2's own edges are removed)")
	}
	if hasEdgeBetween(g, m3.ID, m2.ID) {
		t.Fatal("expected the M3-M2 edge extracted (all of M2's own edges are removed)")
	}
	if hasEdgeBetween(g, m3.ID, d.ID) {
		t.Fatal("expected no M3-D heal edge (unhealable corner topology)")
	}
	if hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("expected the A-B edge to be gone (stitched away by the move)")
	}
	if !hasEdgeBetween(g, a.ID, m2.ID) || !hasEdgeBetween(g, m2.ID, b.ID) {
		t.Fatal("expected stitched A-M2 and M2-B edges")
	}
}

// TestMoveMode_MoveAlongOwnChain (spec #5, interactive): A(0,0)->N(0,4)->B(0,8).
// Arming move on N and tapping (0,6) — interior of N's own N-B edge, and also
// interior of what becomes the healed A-B span — must NOT leave an unsplit
// A->B edge: the heal immediately re-splices at N's new position since
// nodesOnSpan naturally includes the moved node once it lands on the healed
// span. A single undo restores N to (0,4) with the original A->N->B chain.
func TestMoveMode_MoveAlongOwnChain(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n := g.tryAddNode(0, 4, model.NodeTypeRegular)
	b := g.tryAddNode(0, 8, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)

	origI, origJ := n.I, n.J
	nID, aID, bID := n.ID, a.ID, b.ID

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 0, 6)
	g.handleTapInGrid(tx, ty)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected move committed immediately, got mode=%v", g.modes.Active())
	}
	moved := g.nodeAt(0, 6)
	if moved == nil || moved.ID != nID {
		t.Fatalf("expected N at (0,6), got node=%v", moved)
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) {
		t.Fatal("expected A->N and N->B restored via heal+resplice at N's new position")
	}
	if hasDirectedEdge(g, aID, bID) {
		t.Fatal("expected no unsplit A->B edge to survive")
	}

	g.performUndo()

	restored := g.nodeAt(origI, origJ)
	if restored == nil || restored.ID != nID {
		t.Fatalf("undo did not restore N to (%d,%d)", origI, origJ)
	}
	if g.nodeAt(0, 6) != nil {
		t.Fatal("undo left a node behind at the moved-to cell (0,6)")
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) {
		t.Fatal("undo did not restore the original A->N/N->B chain")
	}
	if hasDirectedEdge(g, aID, bID) {
		t.Fatal("undo left an unsplit A->B heal edge behind")
	}
}

// TestMoveMode_SingleUndoStep (extended for Task 2): M carries its own
// pass-through chain P->M->Q (orthogonal, off the A-B row) in addition to the
// shared destination A-B edge, so the move exercises BOTH heal (P-Q closes
// behind M) and destination splice (A-B splits around M) in the same
// undo group. One undo must revert everything: M back at its origin, its own
// P-M/M-Q edges restored, the P-Q heal edge gone, and the A-B destination
// edge restored unsplit.
func TestMoveMode_SingleUndoStep(t *testing.T) {
	g, a, b, m := buildMoveModeLayout(t)

	// Give M its own pass-through chain P(10,-5)->M(10,0)->Q(10,5), orthogonal
	// on the I=10 column, distinct from the A-B edge M is about to splice
	// into.
	p := g.tryAddNode(10, -5, model.NodeTypeRegular)
	q := g.tryAddNode(10, 5, model.NodeTypeRegular)
	g.addEdge(p, m)
	g.addEdge(m, q)
	g.updateBeatInfos()

	origI, origJ := m.I, m.J
	mID, aID, bID := m.ID, a.ID, b.ID
	pID, qID := p.ID, q.ID

	g.modes.Enter(GridModeMove, m)
	tx, ty := screenPosForGridIJ(g, 0, 10)
	g.handleTapInGrid(tx, ty)

	if moved := g.nodeAt(0, 10); moved == nil || moved.ID != mID {
		t.Fatal("setup: expected M committed at (0,10) before testing undo")
	}
	if !hasDirectedEdge(g, pID, qID) {
		t.Fatal("setup: expected the P->Q heal edge to exist after the move")
	}
	if hasEdgeBetween(g, pID, mID) || hasEdgeBetween(g, mID, qID) {
		t.Fatal("setup: expected M's own P-M/M-Q edges extracted")
	}

	g.performUndo()

	restored := g.nodeAt(origI, origJ)
	if restored == nil || restored.ID != mID {
		t.Fatalf("undo did not restore M to (%d,%d)", origI, origJ)
	}
	if g.nodeAt(0, 10) != nil {
		t.Fatal("undo left a node behind at the moved-to cell (0,10)")
	}
	if !hasEdgeBetween(g, aID, bID) {
		t.Fatal("undo did not restore the A-B edge")
	}
	if hasEdgeBetween(g, aID, mID) || hasEdgeBetween(g, mID, bID) {
		t.Fatal("undo did not revert the stitch (A-M / M-B edges still present)")
	}
	if hasDirectedEdge(g, pID, qID) {
		t.Fatal("undo did not remove the P->Q heal edge")
	}
	if !hasDirectedEdge(g, pID, mID) || !hasDirectedEdge(g, mID, qID) {
		t.Fatal("undo did not restore M's own P->M / M->Q edges")
	}
}

func TestMoveMode_EventSequence(t *testing.T) {
	t.Run("valid placement", func(t *testing.T) {
		armed := make(chan hooks.GridModePayload, 4)
		moved := make(chan hooks.NodeEdit, 4)
		canceled := make(chan hooks.GridModePayload, 4)
		u1 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				armed <- p
			}
		})
		defer u1()
		u2 := hooks.GlobalBus().Subscribe(hooks.EventNodeMoved, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.NodeEdit); ok {
				moved <- p
			}
		})
		defer u2()
		u3 := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				canceled <- p
			}
		})
		defer u3()

		g, _, _, m := buildMoveModeLayout(t)
		g.modes.Enter(GridModeMove, m)
		select {
		case <-armed:
		case <-time.After(time.Second):
			t.Fatal("no EventGridModeArmed")
		}

		tx, ty := screenPosForGridIJ(g, 0, 10)
		g.handleTapInGrid(tx, ty)

		select {
		case <-moved:
		case <-time.After(time.Second):
			t.Fatal("no EventNodeMoved")
		}
		select {
		case p := <-canceled:
			t.Fatalf("unexpected EventGridModeCanceled: %+v", p)
		case <-time.After(150 * time.Millisecond):
		}
	})

	t.Run("invalid placement", func(t *testing.T) {
		armed := make(chan hooks.GridModePayload, 4)
		moved := make(chan hooks.NodeEdit, 4)
		canceled := make(chan hooks.GridModePayload, 4)
		u1 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				armed <- p
			}
		})
		defer u1()
		u2 := hooks.GlobalBus().Subscribe(hooks.EventNodeMoved, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.NodeEdit); ok {
				moved <- p
			}
		})
		defer u2()
		u3 := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				canceled <- p
			}
		})
		defer u3()

		g, _, _, m := buildMoveModeLayout(t)
		g.modes.Enter(GridModeMove, m)
		select {
		case <-armed:
		case <-time.After(time.Second):
			t.Fatal("no EventGridModeArmed")
		}

		tx, ty := screenPosForGridIJ(g, 50, 50) // free cell, not on an edge
		g.handleTapInGrid(tx, ty)

		select {
		case p := <-canceled:
			if p.Reason != "invalid-target" {
				t.Fatalf("cancel reason = %q, want invalid-target", p.Reason)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventGridModeCanceled")
		}
		select {
		case p := <-moved:
			t.Fatalf("unexpected EventNodeMoved: %+v", p)
		case <-time.After(150 * time.Millisecond):
		}
	})
}

// ─── Task 5: blink + ghost validity rendering ──────────────────────────────
//
// These tests drive the REAL drawGridPane dispatch (mirrors
// grid_coord_badge_zorder_test.go's TestCoordBadgeDrawnBeforeSidebarPanel
// pattern) and intercept the package-level drawRect var to capture what was
// actually painted — never a manual/isolated draw call, and never a raw
// screen.At() pixel read (that panics under real Ebiten; interception is the
// stub-safe, delta-measuring approach used throughout this package).

// moveModeRectCall records one drawRect invocation (filled or outline) during
// a drawGridMoveMode pass.
type moveModeRectCall struct {
	Rect   image.Rectangle
	Color  color.RGBA
	Filled bool
}

// collectMoveModeRects intercepts drawRect during fn() and returns every call
// in invocation order, filled and outline alike — TestMoveModeGhostValidityStates
// needs the outline-only (filled=false) calls too, which the existing
// collectFilledRects helper (render_verify_test.go) discards.
func collectMoveModeRects(t *testing.T, fn func()) []moveModeRectCall {
	t.Helper()
	var calls []moveModeRectCall
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, moveModeRectCall{
			Rect:   r,
			Color:  color.RGBAModel.Convert(c).(color.RGBA),
			Filled: filled,
		})
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	fn()
	return calls
}

// filledAreaOfColor sums the pixel area of every FILLED call matching col
// whose rect intersects inside. Used as the "filled glow pixel count" proxy:
// drawRect's fast path blits a single filled rect per call (drawRectBlit is
// the un-intercepted primitive), so one matching call's intersected area is
// exactly the filled coverage contributed by that draw.
func filledAreaOfColor(calls []moveModeRectCall, col color.Color, inside image.Rectangle) int {
	want := color.RGBAModel.Convert(col).(color.RGBA)
	area := 0
	for _, c := range calls {
		if !c.Filled || c.Color != want {
			continue
		}
		ir := c.Rect.Intersect(inside)
		if ir.Empty() {
			continue
		}
		area += ir.Dx() * ir.Dy()
	}
	return area
}

// outlineCountOfColor counts OUTLINE (filled=false) calls matching col whose
// rect intersects inside.
func outlineCountOfColor(calls []moveModeRectCall, col color.Color, inside image.Rectangle) int {
	want := color.RGBAModel.Convert(col).(color.RGBA)
	n := 0
	for _, c := range calls {
		if c.Filled || c.Color != want {
			continue
		}
		if c.Rect.Intersect(inside).Empty() {
			continue
		}
		n++
	}
	return n
}

// TestMoveModeBlinkAlternates arms move mode on M, draws once at phase 0
// (blink ON per gridModeManager.blinkOn: (0/gridModeBlinkHalfPeriod)%2==0)
// and once after ticking the phase past gridModeBlinkHalfPeriod (blink OFF),
// and asserts the subject node's screen rect is filled with the blink glow
// color strictly more in the ON draw than the OFF draw.
func TestMoveModeBlinkAlternates(t *testing.T) {
	g, _, _, m := buildMoveModeLayout(t)
	g.modes.Enter(GridModeMove, m)

	mx1, my1, mx2, my2 := g.nodeScreenRect(m)
	subjectRect := image.Rect(int(mx1)-2, int(my1)-2, int(mx2)+2, int(my2)+2)

	blinkFill := WithAlpha(genColorVizGlow, genAlphaMedium)
	img := ebiten.NewImage(800, 600)

	if !g.modes.blinkOn() {
		t.Fatal("setup: expected blinkOn()==true at phase 0")
	}
	onCalls := collectMoveModeRects(t, func() { g.drawGridPane(img) })
	onArea := filledAreaOfColor(onCalls, blinkFill, subjectRect)
	if onArea <= 0 {
		t.Fatalf("expected blink-ON draw to fill the subject rect with the glow color, got area=%d", onArea)
	}

	for i := 0; i < gridModeBlinkHalfPeriod; i++ {
		g.modes.Tick()
	}
	if g.modes.blinkOn() {
		t.Fatal("setup: expected blinkOn()==false after gridModeBlinkHalfPeriod ticks")
	}
	offCalls := collectMoveModeRects(t, func() { g.drawGridPane(img) })
	offArea := filledAreaOfColor(offCalls, blinkFill, subjectRect)

	if !(onArea > offArea) {
		t.Fatalf("expected ON blink area > OFF blink area inside subject rect, got on=%d off=%d", onArea, offArea)
	}
}

// TestMoveModeGhostValidityStates arms move mode on M, then draws with the
// cursor over an interior point of the A-B edge (valid target, GuideEdge) and
// separately over a far-away free cell (invalid target, not GuideEdge).
// Valid must produce a filled glow ghost; invalid must produce strictly fewer
// filled-glow pixels (zero) plus a nonzero outline-only ghost.
func TestMoveModeGhostValidityStates(t *testing.T) {
	g, _, _, m := buildMoveModeLayout(t)
	g.modes.Enter(GridModeMove, m)

	mx, my := new(int), new(int)
	restore := SetInputForTest(
		func() (int, int) { return *mx, *my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)

	img := ebiten.NewImage(800, 600)
	ghostFill := WithAlpha(genColorVizGlow, genAlphaAccentOverlay)
	ghostOutline := WithAlpha(genColorVizGlow, genAlphaSubtle)
	fullFrame := image.Rect(0, 0, 800, 600)

	// centerCameraOn (render_verify_test.go) re-centers the camera so grid
	// (gi,gj) maps to the grid pane's on-screen center — always inside the
	// pane's visible bounds regardless of shrinkGuideSnapRadius's huge scale
	// (a raw screenPosForGridIJ(g, 0, 10) screen point lands far outside the
	// 800x600 window at that scale, so g.split.InGridPane would reject it and
	// the whole ghost block would be skipped for the wrong reason).
	paneCenterX := g.winW / 2
	paneCenterY := gridTopOffset() + (g.split.Y-gridTopOffset())/2

	// Valid target: interior point of the A-B edge.
	centerCameraOn(g, 0, 10)
	*mx, *my = paneCenterX, paneCenterY
	if gt := g.guideTargetAt(paneCenterX, paneCenterY); gt.Kind != GuideEdge {
		t.Fatalf("setup: expected cursor over (0,10) to resolve to GuideEdge, got %v", gt.Kind)
	}
	validCalls := collectMoveModeRects(t, func() { g.drawGridPane(img) })
	validFillArea := filledAreaOfColor(validCalls, ghostFill, fullFrame)
	if validFillArea <= 0 {
		t.Fatalf("expected a filled glow ghost over a valid (GuideEdge) target, got area=%d", validFillArea)
	}

	// Invalid target: a free cell far from any node/edge.
	centerCameraOn(g, 50, 50)
	*mx, *my = paneCenterX, paneCenterY
	if gt := g.guideTargetAt(paneCenterX, paneCenterY); gt.Kind == GuideEdge {
		t.Fatalf("setup: expected cursor over (50,50) to NOT resolve to GuideEdge, got %v", gt.Kind)
	}
	invalidCalls := collectMoveModeRects(t, func() { g.drawGridPane(img) })
	invalidFillArea := filledAreaOfColor(invalidCalls, ghostFill, fullFrame)
	invalidOutlineCount := outlineCountOfColor(invalidCalls, ghostOutline, fullFrame)

	if !(validFillArea > invalidFillArea) {
		t.Fatalf("expected valid-target filled ghost area > invalid-target filled ghost area, got valid=%d invalid=%d",
			validFillArea, invalidFillArea)
	}
	if invalidFillArea != 0 {
		t.Fatalf("expected NO filled ghost over an invalid target, got area=%d", invalidFillArea)
	}
	if invalidOutlineCount == 0 {
		t.Fatal("expected a nonzero outline-only ghost over an invalid target")
	}
}

// ─── Task 6: long-press popup lifecycle + option-tap telemetry ────────────

// TestLongPressPopupEmitsLifecycleEvents drives the real showLongPressPopup /
// updateLongPressPopup entry points and asserts the popup.opened /
// popup.closed / ui.tap / mode.armed event sequence for both the Move
// activation path and the outside-release dismiss path.
func TestLongPressPopupEmitsLifecycleEvents(t *testing.T) {
	t.Run("move activation", func(t *testing.T) {
		opened := make(chan hooks.PopupPayload, 4)
		closed := make(chan hooks.PopupPayload, 4)
		tapped := make(chan hooks.UITapPayload, 4)
		armed := make(chan hooks.GridModePayload, 4)
		u1 := hooks.GlobalBus().Subscribe(hooks.EventPopupOpened, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.PopupPayload); ok {
				opened <- p
			}
		})
		defer u1()
		u2 := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.PopupPayload); ok {
				closed <- p
			}
		})
		defer u2()
		u3 := hooks.GlobalBus().Subscribe(hooks.EventUITap, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.UITapPayload); ok {
				tapped <- p
			}
		})
		defer u3()
		u4 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				armed <- p
			}
		})
		defer u4()

		assertDefaultParityState(t)
		withDefaultStart(t, false)
		withSmallScreen(t, true)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(390, 844)

		n := g.tryAddNode(4, 0, model.NodeTypeRegular)
		sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
		cx := int((sx1 + sx2) * 0.5)
		cy := int((sy1 + sy2) * 0.5)
		g.handleTouchLongPress(cx, cy)

		select {
		case p := <-opened:
			if p.ID != "node-longpress" {
				t.Fatalf("popup.opened ID = %q, want node-longpress", p.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventPopupOpened")
		}

		mcx := (g.longPressPopupMove.Min.X + g.longPressPopupMove.Max.X) / 2
		mcy := (g.longPressPopupMove.Min.Y + g.longPressPopupMove.Max.Y) / 2
		g.leftPrev = true
		g.updateLongPressPopup(mcx, mcy, true)  // hover
		g.updateLongPressPopup(mcx, mcy, false) // release → activate Move

		select {
		case p := <-tapped:
			if p.Label != "Move" {
				t.Fatalf("ui.tap Label = %q, want Move", p.Label)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventUITap")
		}
		select {
		case p := <-closed:
			if p.ID != "node-longpress" {
				t.Fatalf("popup.closed ID = %q, want node-longpress", p.ID)
			}
			if p.Reason != "action" {
				t.Fatalf("popup.closed Reason = %q, want action", p.Reason)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventPopupClosed")
		}
		select {
		case p := <-armed:
			if p.Mode != "move" {
				t.Fatalf("mode.armed Mode = %q, want move", p.Mode)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventGridModeArmed")
		}
	})

	t.Run("outside release dismiss", func(t *testing.T) {
		closed := make(chan hooks.PopupPayload, 4)
		tapped := make(chan hooks.UITapPayload, 4)
		armed := make(chan hooks.GridModePayload, 4)
		u1 := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.PopupPayload); ok {
				closed <- p
			}
		})
		defer u1()
		u2 := hooks.GlobalBus().Subscribe(hooks.EventUITap, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.UITapPayload); ok {
				tapped <- p
			}
		})
		defer u2()
		u3 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
			if p, ok := e.Payload.(hooks.GridModePayload); ok {
				armed <- p
			}
		})
		defer u3()

		assertDefaultParityState(t)
		withDefaultStart(t, false)
		withSmallScreen(t, true)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(390, 844)

		n := g.tryAddNode(4, 0, model.NodeTypeRegular)
		sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
		cx := int((sx1 + sx2) * 0.5)
		cy := int((sy1 + sy2) * 0.5)
		g.handleTouchLongPress(cx, cy)

		// Release off all buttons.
		g.leftPrev = true
		g.updateLongPressPopup(0, 0, true)
		g.updateLongPressPopup(0, 0, false)

		select {
		case p := <-closed:
			if p.Reason != "dismiss" {
				t.Fatalf("popup.closed Reason = %q, want dismiss", p.Reason)
			}
		case <-time.After(time.Second):
			t.Fatal("no EventPopupClosed")
		}
		select {
		case p := <-tapped:
			t.Fatalf("unexpected EventUITap: %+v", p)
		case <-time.After(150 * time.Millisecond):
		}
		select {
		case p := <-armed:
			t.Fatalf("unexpected EventGridModeArmed: %+v", p)
		case <-time.After(150 * time.Millisecond):
		}
	})
}

// ─── Task 7: Copy option + armed copy mode ─────────────────────────────────

// TestLongPressPopupCopyArmsCopyMode: open the popup, slide-release on the
// Copy button. Expect the clipboard to hold the node's type/params, copy
// mode armed, EventNodeCopied and EventGridModeArmed{Mode:"copy"} emitted.
func TestLongPressPopupCopyArmsCopyMode(t *testing.T) {
	copied := make(chan hooks.NodeEdit, 4)
	armed := make(chan hooks.GridModePayload, 4)
	u1 := hooks.GlobalBus().Subscribe(hooks.EventNodeCopied, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.NodeEdit); ok {
			copied <- p
		}
	})
	defer u1()
	u2 := hooks.GlobalBus().Subscribe(hooks.EventGridModeArmed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			armed <- p
		}
	})
	defer u2()

	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleTouchLongPress(cx, cy)
	if !g.longPressPopup {
		t.Fatal("setup: expected popup open")
	}
	if g.longPressPopupCopy.Empty() {
		t.Fatal("expected copy button rect to be non-empty")
	}

	ccx := (g.longPressPopupCopy.Min.X + g.longPressPopupCopy.Max.X) / 2
	ccy := (g.longPressPopupCopy.Min.Y + g.longPressPopupCopy.Max.Y) / 2
	g.leftPrev = true
	g.updateLongPressPopup(ccx, ccy, true) // hover
	if g.longPressPopupHover != "copy" {
		t.Fatalf("expected hover='copy', got %q", g.longPressPopupHover)
	}
	g.updateLongPressPopup(ccx, ccy, false) // release → activate Copy

	if g.longPressPopup {
		t.Fatal("popup should be dismissed after action")
	}
	if !g.modes.Is(GridModeCopy) {
		t.Fatal("expected GridModeCopy armed")
	}
	if g.modes.Subject() != n {
		t.Fatal("expected copy subject to be the long-pressed node")
	}
	if !g.clipboard.has {
		t.Fatal("expected clipboard.has == true")
	}
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("setup: expected node in graph")
	}
	if g.clipboard.typ != mn.Type {
		t.Fatalf("clipboard type = %v, want %v", g.clipboard.typ, mn.Type)
	}
	if g.clipboard.params.Volume != mn.Params.Volume {
		t.Fatalf("clipboard volume = %v, want %v", g.clipboard.params.Volume, mn.Params.Volume)
	}

	select {
	case p := <-copied:
		if p.ID != int(n.ID) {
			t.Fatalf("EventNodeCopied ID = %d, want %d", p.ID, n.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventNodeCopied")
	}
	select {
	case p := <-armed:
		if p.Mode != "copy" {
			t.Fatalf("EventGridModeArmed Mode = %q, want copy", p.Mode)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeArmed")
	}
}

// buildCopyModeLayout builds a small circuit for copy-mode tap tests: node M
// (the copy subject, with a distinctive Volume so paste-preserves-params is
// verifiable), an A-B edge with interior cells, and node B as the "occupied"
// target.
func buildCopyModeLayout(t *testing.T) (g *Game, a, b, m *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(0, 20, model.NodeTypeRegular)
	g.addEdge(a, b)
	m = g.tryAddNode(10, 0, model.NodeTypeRegular)

	// Give M a distinctive Volume so paste-preserves-params is verifiable.
	mn, ok := g.graph.GetNodeByID(m.ID)
	if !ok {
		t.Fatal("setup: expected M in graph")
	}
	mn.Params.Volume = 0.37
	g.graph.Nodes[m.ID] = mn

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)
	return g, a, b, m
}

// TestCopyMode_TapFreeCellPastes arms copy on M then taps a free cell (10,4):
// a new node must exist there with M's type+params, M itself must be
// unchanged, the mode must commit (None), EventNodePasted must fire, and the
// node count must increase by exactly one.
func TestCopyMode_TapFreeCellPastes(t *testing.T) {
	pasted := make(chan hooks.NodeEdit, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventNodePasted, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.NodeEdit); ok {
			pasted <- p
		}
	})
	defer u()

	g, _, _, m := buildCopyModeLayout(t)
	mn, ok := g.graph.GetNodeByID(m.ID)
	if !ok {
		t.Fatal("setup: expected M in graph")
	}
	nodesBefore := len(g.nodes)

	g.snapshotNodeToClipboard(m)
	g.modes.Enter(GridModeCopy, m)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode committed (None), got %v", g.modes.Active())
	}
	pastedNode := g.nodeAt(10, 4)
	if pastedNode == nil {
		t.Fatal("expected a new node at (10,4)")
	}
	if pastedNode.ID == m.ID {
		t.Fatal("expected a fresh node ID for the paste, not M's own ID")
	}
	pn, ok := g.graph.GetNodeByID(pastedNode.ID)
	if !ok {
		t.Fatal("expected pasted node in graph")
	}
	if pn.Type != mn.Type {
		t.Fatalf("pasted type = %v, want %v", pn.Type, mn.Type)
	}
	if pn.Params.Volume != mn.Params.Volume {
		t.Fatalf("pasted volume = %v, want %v", pn.Params.Volume, mn.Params.Volume)
	}

	// M itself must be unchanged.
	if m.I != 10 || m.J != 0 {
		t.Fatalf("M moved unexpectedly: (%d,%d)", m.I, m.J)
	}
	mnAfter, _ := g.graph.GetNodeByID(m.ID)
	if mnAfter.Params.Volume != 0.37 {
		t.Fatalf("M's own volume changed: %v", mnAfter.Params.Volume)
	}

	if len(g.nodes) != nodesBefore+1 {
		t.Fatalf("node count = %d, want %d", len(g.nodes), nodesBefore+1)
	}

	select {
	case p := <-pasted:
		if p.I != 10 || p.J != 4 {
			t.Fatalf("EventNodePasted at (%d,%d), want (10,4)", p.I, p.J)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventNodePasted")
	}
}

// TestCopyMode_TapOnEdgeCellPastesAndStitches arms copy on M then taps an
// interior point of the A-B edge: the pasted node must be stitched into that
// edge exactly like pasteNodeFrom's manual-add semantics (A-B edge replaced
// by A-paste and paste-B).
func TestCopyMode_TapOnEdgeCellPastesAndStitches(t *testing.T) {
	g, a, b, m := buildCopyModeLayout(t)
	if !hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("setup: expected A-B edge before paste")
	}

	g.snapshotNodeToClipboard(m)
	g.modes.Enter(GridModeCopy, m)
	tx, ty := screenPosForGridIJ(g, 0, 10) // interior point of the A-B edge
	g.handleTapInGrid(tx, ty)

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode committed (None), got %v", g.modes.Active())
	}
	pastedNode := g.nodeAt(0, 10)
	if pastedNode == nil {
		t.Fatal("expected a new node at (0,10)")
	}
	if hasEdgeBetween(g, a.ID, b.ID) {
		t.Fatal("expected the A-B edge to be gone (stitched away)")
	}
	if !hasEdgeBetween(g, a.ID, pastedNode.ID) {
		t.Fatal("expected a stitched A-paste edge")
	}
	if !hasEdgeBetween(g, pastedNode.ID, b.ID) {
		t.Fatal("expected a stitched paste-B edge")
	}
}

// TestCopyMode_TapOccupiedOpensOverwriteConfirm arms copy on M then taps
// occupied node B: the overwrite confirmation portal must own the flow (the
// full ConfirmPasteOverwrite path is covered by node_clipboard_test.go), and
// the armed mode's job is done (committed).
func TestCopyMode_TapOccupiedOpensOverwriteConfirm(t *testing.T) {
	g, _, b, m := buildCopyModeLayout(t)

	g.snapshotNodeToClipboard(m)
	g.modes.Enter(GridModeCopy, m)
	bx, by := nodeScreenCenter(g, b)
	g.handleTapInGrid(bx, by)

	if !g.pasteConfirmActive() {
		t.Fatal("expected paste-overwrite confirm to be active")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode committed (None), got %v", g.modes.Active())
	}
}

// TestCopyMode_EscCancels arms copy mode then presses ESC via handleEscape
// (the single Esc authority, per the Task 6 consolidation): mode must go to
// None and EventGridModeCanceled{Mode:"copy", Reason:"esc"} must fire.
func TestCopyMode_EscCancels(t *testing.T) {
	canceled := make(chan hooks.GridModePayload, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u()

	g, _, _, m := buildCopyModeLayout(t)
	g.snapshotNodeToClipboard(m)
	g.modes.Enter(GridModeCopy, m)

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleEscape()

	if g.modes.Is(GridModeCopy) {
		t.Fatal("expected GridModeCopy canceled")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode None, got %v", g.modes.Active())
	}

	select {
	case p := <-canceled:
		if p.Mode != "copy" {
			t.Fatalf("EventGridModeCanceled Mode = %q, want copy", p.Mode)
		}
		if p.Reason != "esc" {
			t.Fatalf("EventGridModeCanceled Reason = %q, want esc", p.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeCanceled")
	}
}

// --- Task 8: desktop mouse click-and-hold opens the long-press popup -------
//
// All five tests drive the real handleEditor entry point via g.Update(),
// mirroring TestClickNodeByScreenRectOpensPopup's press/release-across-frames
// pattern (node_click_screen_hit_test.go): SetInputForTest wires mutable
// mx/my/left closures, and successive g.Update() calls advance one frame each.
// touchOverrideActive is left at its SetInputForTest-reset default (false) —
// this is the desktop mouse path, not touch.

// mouseHoldDriver wires SetInputForTest with mutable cursor/button state and
// returns setters plus the restore func, so each test can drive frames with
// plain assignments instead of re-deriving the closures every time.
func mouseHoldDriver(g *Game) (setPos func(x, y int), setLeft func(bool), restore func()) {
	mx, my := 0, 0
	left := false
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	setPos = func(x, y int) { mx, my = x, y }
	setLeft = func(v bool) { left = v }
	return setPos, setLeft, restore
}

// TestMouseHoldOnNodeOpensPopup verifies that pressing and holding the mouse
// stationary on a node for mouseHoldPopupFrames+1 frames opens the long-press
// quick-action popup for that node, and that the hold consumes the press so
// the eventual release does NOT also open the node sidebar.
func TestMouseHoldOnNodeOpensPopup(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	cx, cy := nodeScreenCenter(g, n)

	setPos, setLeft, restore := mouseHoldDriver(g)
	defer restore()

	setPos(cx, cy)
	setLeft(true)
	for i := 0; i < mouseHoldPopupFrames+1; i++ {
		_ = g.Update()
	}

	if !g.longPressPopup {
		t.Fatal("expected longPressPopup to be true after holding on a node")
	}
	if g.longPressPopupNode != n {
		t.Fatalf("expected popup node = %v, got %v", n, g.longPressPopupNode)
	}

	// Release: since the hold consumed pendingClick, this must NOT open the
	// sidebar (the popup intercept owns input while g.longPressPopup is true,
	// so handleEditor's release branch never even runs — the sidebar staying
	// closed is the observable proof the click was consumed).
	setLeft(false)
	_ = g.Update()

	if g.sidebar.IsOpen() {
		t.Fatal("sidebar should not open after a mouse-hold that opened the popup")
	}
}

// TestMouseHoldCanceledByDrag verifies that moving the cursor beyond
// tapMaxMovePx during the hold kills the hold permanently for that press —
// the popup never opens even if the button stays down well past the
// threshold.
func TestMouseHoldCanceledByDrag(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	cx, cy := nodeScreenCenter(g, n)

	setPos, setLeft, restore := mouseHoldDriver(g)
	defer restore()

	setPos(cx, cy)
	setLeft(true)
	for i := 0; i < 10; i++ {
		_ = g.Update()
	}
	if g.longPressPopup {
		t.Fatal("popup should not have opened yet after only 10 frames")
	}

	// Drag well past the tap-move threshold, then keep holding past the
	// point where the hold would otherwise have fired.
	setPos(cx+tapMaxMovePx+20, cy)
	for i := 0; i < mouseHoldPopupFrames+20; i++ {
		_ = g.Update()
	}

	if g.longPressPopup {
		t.Fatal("popup should never open once the hold was canceled by drag")
	}

	// Release: leaving the button "held" past the end of the test would leak
	// the camera's package-level last-mouse-position tracker (camera.go's
	// lastMouse) into whichever test runs next, making that test's very
	// first press frame look like a continuing drag. Always end a
	// still-held test with a release.
	setLeft(false)
	_ = g.Update()
}

// TestMouseShortClickStillOpensSidebar verifies existing click-to-select
// behavior is unaffected by the new hold detector: a quick press+release
// well under the hold threshold still opens the node sidebar.
func TestMouseShortClickStillOpensSidebar(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	cx, cy := nodeScreenCenter(g, n)

	setPos, setLeft, restore := mouseHoldDriver(g)
	defer restore()

	setPos(cx, cy)
	setLeft(true)
	_ = g.Update()
	setLeft(false)
	_ = g.Update()

	if g.longPressPopup {
		t.Fatal("a quick click must not open the long-press popup")
	}
	if !g.sidebar.IsOpen() || g.sidebar.Node() != n {
		t.Fatal("expected sidebar open for the clicked node")
	}
}

// TestMouseHoldThenSlideReleaseArmsMove verifies that once the popup is open
// via a mouse hold, the existing slide-to-button-then-release activation
// (shared with touch via updateLongPressPopup) works with the mouse: sliding
// into the Move button rect and releasing arms GridModeMove.
func TestMouseHoldThenSlideReleaseArmsMove(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	cx, cy := nodeScreenCenter(g, n)

	setPos, setLeft, restore := mouseHoldDriver(g)
	defer restore()

	setPos(cx, cy)
	setLeft(true)
	for i := 0; i < mouseHoldPopupFrames+1; i++ {
		_ = g.Update()
	}
	if !g.longPressPopup {
		t.Fatal("expected popup open after hold")
	}

	// Slide into the Move button (finger/button still down) — the intercept
	// in game_update.go routes to updateLongPressPopup while longPressPopup
	// is true, so this hover-tracking frame works identically to touch.
	mvx := (g.longPressPopupMove.Min.X + g.longPressPopupMove.Max.X) / 2
	mvy := (g.longPressPopupMove.Min.Y + g.longPressPopupMove.Max.Y) / 2
	setPos(mvx, mvy)
	_ = g.Update()

	setLeft(false)
	_ = g.Update()

	if g.longPressPopup {
		t.Fatal("popup should be dismissed after the Move action")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatal("expected GridModeMove armed after slide-release on Move")
	}
	if g.modes.Subject() != n {
		t.Fatalf("expected move subject = %v, got %v", n, g.modes.Subject())
	}
}

// TestMouseHoldOnEmptySpaceDoesNothing verifies that holding the mouse on
// empty grid space (no node under the press) never opens the popup — the
// hold detector gates on g.clickNode != nil.
func TestMouseHoldOnEmptySpaceDoesNothing(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Nothing placed near this grid coordinate.
	ex, ey := screenPosForGridIJ(g, 30, 30)

	setPos, setLeft, restore := mouseHoldDriver(g)
	defer restore()

	setPos(ex, ey)
	setLeft(true)
	for i := 0; i < mouseHoldPopupFrames+1; i++ {
		_ = g.Update()
	}

	if g.longPressPopup {
		t.Fatal("popup should not open when holding on empty grid space")
	}

	// Release: see the comment in TestMouseHoldCanceledByDrag — a test that
	// ends still holding the button leaks camera.go's package-level
	// lastMouse tracker into the next test's first press frame.
	setLeft(false)
	_ = g.Update()
}

// TestMoveMode_SubjectDeletedCancelsMode: arming move mode on a node and then
// deleting that same node (e.g. via the sidebar's delete action, which can
// fire while a mode is armed on a different node reference than the one being
// deleted — or, degenerate case, the armed subject itself) must not leave the
// mode manager pointing at a dangling *uiNode. pruneDanglingNodeRefs runs once
// per Update (game_update.go) and on Import; this drives it the same way the
// real frame loop does.
func TestMoveMode_SubjectDeletedCancelsMode(t *testing.T) {
	g, _, _, m := buildMoveModeLayout(t)

	g.modes.Enter(GridModeMove, m)
	if g.modes.Active() != GridModeMove {
		t.Fatalf("setup: expected move mode armed, got %v", g.modes.Active())
	}
	g.moveSkipRelease = true

	g.deleteNode(m)
	if _, ok := g.nodesByID[m.ID]; ok {
		t.Fatal("setup: expected deleted node to be gone from nodesByID")
	}

	g.pruneDanglingNodeRefs()

	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled after subject deletion, got %v", g.modes.Active())
	}
	if g.modes.Subject() != nil {
		t.Fatalf("expected nil subject after cancel, got %v", g.modes.Subject())
	}
	if g.moveSkipRelease {
		t.Fatal("expected moveSkipRelease cleared after subject-deleted cancel")
	}
}

// --- Task 3: "Break the circuit?" confirm portal ---------------------------
//
// Square circuit fixture for the corner/junction/double-in/double-out cases:
// A(0,0)->N(0,6)->B(6,6)->C(6,0)->A, plus a separate target edge
// X(10,0)->Y(10,8) supplying a valid GuideEdge destination at (10,4).

// buildBreakConfirmSquareLayout builds the shared square A->N->B->C->A plus
// the X-Y target edge, with the guide-snap radius shrunk for deterministic
// tap resolution. N is the corner subject: in=A (vertical), out=B
// (horizontal) — perpendicular, so moveHealPlan classifies N healUnhealable.
func buildBreakConfirmSquareLayout(t *testing.T) (g *Game, a, n, b, c, x, y *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	n = g.tryAddNode(0, 6, model.NodeTypeRegular)
	b = g.tryAddNode(6, 6, model.NodeTypeRegular)
	c = g.tryAddNode(6, 0, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	g.addEdge(b, c)
	g.addEdge(c, a)

	x = g.tryAddNode(10, 0, model.NodeTypeRegular)
	y = g.tryAddNode(10, 8, model.NodeTypeRegular)
	g.addEdge(x, y)

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)
	return g, a, n, b, c, x, y
}

// buildBreakConfirmJunctionLayout builds A->N->B plus a second outgoing
// N->D, so N carries 3 total edges (1 in, 2 out) — always healUnhealable
// regardless of orthogonality (moveHealPlan only pass-throughs exactly 1
// in + 1 out). Plus the X-Y target edge.
func buildBreakConfirmJunctionLayout(t *testing.T) (g *Game, n *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n = g.tryAddNode(0, 6, model.NodeTypeRegular)
	b := g.tryAddNode(0, 12, model.NodeTypeRegular)
	d := g.tryAddNode(6, 6, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(n, b)
	g.addEdge(n, d)

	x := g.tryAddNode(10, 0, model.NodeTypeRegular)
	y := g.tryAddNode(10, 8, model.NodeTypeRegular)
	g.addEdge(x, y)

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)
	return g, n
}

// buildBreakConfirmDoubleInLayout builds A->N, B->N (two incoming, zero
// outgoing) — healUnhealable since moveHealPlan's pass-through branch
// requires exactly 1 in + 1 out. Plus the X-Y target edge.
func buildBreakConfirmDoubleInLayout(t *testing.T) (g *Game, n *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n = g.tryAddNode(0, 6, model.NodeTypeRegular)
	b := g.tryAddNode(6, 6, model.NodeTypeRegular)
	g.addEdge(a, n)
	g.addEdge(b, n)

	x := g.tryAddNode(10, 0, model.NodeTypeRegular)
	y := g.tryAddNode(10, 8, model.NodeTypeRegular)
	g.addEdge(x, y)

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)
	return g, n
}

// buildBreakConfirmDoubleOutLayout builds N->A, N->B (two outgoing, zero
// incoming) — healUnhealable for the same reason as double-in. Plus the X-Y
// target edge.
func buildBreakConfirmDoubleOutLayout(t *testing.T) (g *Game, n *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n = g.tryAddNode(0, 6, model.NodeTypeRegular)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(6, 6, model.NodeTypeRegular)
	g.addEdge(n, a)
	g.addEdge(n, b)

	x := g.tryAddNode(10, 0, model.NodeTypeRegular)
	y := g.tryAddNode(10, 8, model.NodeTypeRegular)
	g.addEdge(x, y)

	g.updateBeatInfos()
	shrinkGuideSnapRadius(g)
	return g, n
}

// TestMoveMode_CornerOpensBreakConfirm (spec #8): arm move on N (corner —
// in=A(0,0), out=B(6,6), perpendicular) and tap the X-Y target edge's
// interior point (10,4): the break-confirm dialog opens, the graph is
// UNCHANGED (all 4 square edges intact, N still at (0,6)), and the mode
// stays armed on N.
func TestMoveMode_CornerOpensBreakConfirm(t *testing.T) {
	g, a, n, b, c, x, y := buildBreakConfirmSquareLayout(t)
	nID, aID, bID, cID, xID, yID := n.ID, a.ID, b.ID, c.ID, x.ID, y.ID

	if plan := g.moveHealPlan(n); plan.class != healUnhealable {
		t.Fatalf("setup: expected corner topology to classify healUnhealable, got %v", plan.class)
	}

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	if !g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog to open on corner move")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatalf("expected move mode still armed, got %v", g.modes.Active())
	}
	moved := g.nodeAt(0, 6)
	if moved == nil || moved.ID != nID {
		t.Fatal("expected N to remain at (0,6) — dialog must not mutate the graph")
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) ||
		!hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("expected all 4 square edges intact while the dialog is open")
	}
	if !hasDirectedEdge(g, xID, yID) {
		t.Fatal("expected the X-Y target edge intact (unsplit) while the dialog is open")
	}
}

// TestMoveMode_BreakConfirmBreak (spec #9): confirming the corner-move
// break-confirm splices N into X-Y (X->N, N->Y), extracts A->N and N->B
// with NO heal edge A->B, and commits the mode. A single undo restores the
// entire square AND unsplits X-Y.
func TestMoveMode_BreakConfirmBreak(t *testing.T) {
	g, a, n, b, c, x, y := buildBreakConfirmSquareLayout(t)
	nID, aID, bID, cID, xID, yID := n.ID, a.ID, b.ID, c.ID, x.ID, y.ID
	origI, origJ := n.I, n.J

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected break-confirm dialog open")
	}

	g.ConfirmBreakCircuit()

	if g.breakConfirmActive() {
		t.Fatal("expected dialog closed after confirm")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode committed (None), got %v", g.modes.Active())
	}
	moved := g.nodeAt(10, 4)
	if moved == nil || moved.ID != nID {
		t.Fatalf("expected N spliced at (10,4), got node=%v", moved)
	}
	if !hasDirectedEdge(g, xID, nID) || !hasDirectedEdge(g, nID, yID) {
		t.Fatal("expected N spliced into X-Y (X->N, N->Y)")
	}
	if hasEdgeBetween(g, xID, yID) {
		t.Fatal("expected the X-Y edge to be gone (split by the splice)")
	}
	if hasEdgeBetween(g, aID, nID) || hasEdgeBetween(g, nID, bID) {
		t.Fatal("expected A-N and N-B extracted (circuit broken)")
	}
	if hasEdgeBetween(g, aID, bID) {
		t.Fatal("expected NO heal edge A-B (unhealable corner)")
	}
	if !hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("expected the B-C and C-A edges untouched")
	}

	g.performUndo()

	restored := g.nodeAt(origI, origJ)
	if restored == nil || restored.ID != nID {
		t.Fatalf("undo did not restore N to (%d,%d)", origI, origJ)
	}
	if g.nodeAt(10, 4) != nil {
		t.Fatal("undo left a node behind at the spliced cell (10,4)")
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) ||
		!hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("undo did not restore the full square")
	}
	if !hasDirectedEdge(g, xID, yID) {
		t.Fatal("undo did not unsplit X-Y")
	}
	if hasEdgeBetween(g, xID, nID) || hasEdgeBetween(g, nID, yID) {
		t.Fatal("undo left the X-N/N-Y splice edges behind")
	}
}

// TestMoveMode_BreakConfirmCancelStaysArmed (spec #10): canceling the dialog
// leaves the graph untouched, the dialog closed, and move mode STILL ARMED
// on N. A follow-up tap on the same target re-opens the dialog (retry
// works). ESC then cancels the dialog first (rung); a second ESC disarms
// the mode.
func TestMoveMode_BreakConfirmCancelStaysArmed(t *testing.T) {
	canceled := make(chan hooks.GridModePayload, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u()

	g, a, n, b, c, _, _ := buildBreakConfirmSquareLayout(t)
	nID, aID, bID, cID := n.ID, a.ID, b.ID, c.ID

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected break-confirm dialog open")
	}

	g.CancelBreakCircuit()

	if g.breakConfirmActive() {
		t.Fatal("expected dialog closed after cancel")
	}
	if !g.modes.Is(GridModeMove) || g.modes.Subject() != n {
		t.Fatalf("expected move mode still armed with subject N, got mode=%v subject=%v", g.modes.Active(), g.modes.Subject())
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) ||
		!hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("expected the square untouched after cancel")
	}

	// Retry: a follow-up tap on the same target re-opens the dialog.
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("expected retry to re-open the dialog")
	}

	// ESC cancels the dialog first (dialog closes, mode stays armed)...
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()
	g.handleEscape()
	if g.breakConfirmActive() {
		t.Fatal("expected first ESC to close the dialog")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatalf("expected move mode still armed after first ESC, got %v", g.modes.Active())
	}

	// ...a second ESC disarms the mode.
	g.handleEscape()
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode disarmed after second ESC, got %v", g.modes.Active())
	}

	select {
	case p := <-canceled:
		if p.Reason != "esc" {
			t.Fatalf("EventGridModeCanceled Reason = %q, want esc", p.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeCanceled after second ESC")
	}
}

// TestMoveMode_JunctionOpensBreakConfirm (spec #11): N with 3 edges → dialog.
func TestMoveMode_JunctionOpensBreakConfirm(t *testing.T) {
	g, n := buildBreakConfirmJunctionLayout(t)
	if plan := g.moveHealPlan(n); plan.class != healUnhealable {
		t.Fatalf("setup: expected junction topology to classify healUnhealable, got %v", plan.class)
	}

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	if !g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog to open on junction move")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatalf("expected move mode still armed, got %v", g.modes.Active())
	}
}

// TestMoveMode_DoubleInOpensBreakConfirm (spec #12): two incoming edges into
// N (zero outgoing) → dialog.
func TestMoveMode_DoubleInOpensBreakConfirm(t *testing.T) {
	g, n := buildBreakConfirmDoubleInLayout(t)
	if plan := g.moveHealPlan(n); plan.class != healUnhealable {
		t.Fatalf("setup: expected double-in topology to classify healUnhealable, got %v", plan.class)
	}

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	if !g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog to open on double-in move")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatalf("expected move mode still armed, got %v", g.modes.Active())
	}
}

// TestMoveMode_DoubleOutOpensBreakConfirm (spec #12, double-out variant):
// two outgoing edges from N (zero incoming) → dialog.
func TestMoveMode_DoubleOutOpensBreakConfirm(t *testing.T) {
	g, n := buildBreakConfirmDoubleOutLayout(t)
	if plan := g.moveHealPlan(n); plan.class != healUnhealable {
		t.Fatalf("setup: expected double-out topology to classify healUnhealable, got %v", plan.class)
	}

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	if !g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog to open on double-out move")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatalf("expected move mode still armed, got %v", g.modes.Active())
	}
}

// TestMoveMode_BreakConfirmEvents (spec #14): the corner tap emits
// popup.opened{ID:"break-circuit-confirm"}; Cancel emits popup.closed and NO
// mode.canceled within 150ms (mode stays armed, not canceled); a re-tap +
// Confirm emits node.moved. Portal Open/Close emit popup events
// automatically via portal.go.
func TestMoveMode_BreakConfirmEvents(t *testing.T) {
	opened := make(chan hooks.PopupPayload, 4)
	closed := make(chan hooks.PopupPayload, 4)
	canceled := make(chan hooks.GridModePayload, 4)
	moved := make(chan hooks.NodeEdit, 4)
	u1 := hooks.GlobalBus().Subscribe(hooks.EventPopupOpened, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			opened <- p
		}
	})
	defer u1()
	u2 := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			closed <- p
		}
	})
	defer u2()
	u3 := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u3()
	u4 := hooks.GlobalBus().Subscribe(hooks.EventNodeMoved, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.NodeEdit); ok {
			moved <- p
		}
	})
	defer u4()

	g, _, n, _, _, _, _ := buildBreakConfirmSquareLayout(t)
	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)

	select {
	case p := <-opened:
		if p.ID != breakConfirmPortalID {
			t.Fatalf("popup.opened ID = %q, want %q", p.ID, breakConfirmPortalID)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventPopupOpened for break-confirm")
	}

	g.CancelBreakCircuit()

	select {
	case p := <-closed:
		if p.ID != breakConfirmPortalID {
			t.Fatalf("popup.closed ID = %q, want %q", p.ID, breakConfirmPortalID)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventPopupClosed after cancel")
	}
	select {
	case p := <-canceled:
		t.Fatalf("unexpected EventGridModeCanceled after Cancel: %+v", p)
	case <-time.After(150 * time.Millisecond):
	}

	// Retry + confirm.
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected re-opened dialog")
	}
	g.ConfirmBreakCircuit()

	select {
	case <-moved:
	case <-time.After(time.Second):
		t.Fatal("no EventNodeMoved after confirm")
	}
}

// TestMoveMode_SubjectDeletedClosesBreakConfirm: with the break-confirm
// dialog open, deleting the armed subject N and running the prune pass must
// close the dialog AND cancel the mode — the prune rung already cancels the
// mode on subject deletion; this extends it to also close the dialog so a
// dangling breakPending.node reference can't linger.
func TestMoveMode_SubjectDeletedClosesBreakConfirm(t *testing.T) {
	g, _, n, _, _, _, _ := buildBreakConfirmSquareLayout(t)
	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected break-confirm dialog open")
	}

	g.deleteNode(n)
	if _, ok := g.nodesByID[n.ID]; ok {
		t.Fatal("setup: expected deleted node gone from nodesByID")
	}

	g.pruneDanglingNodeRefs()

	if g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog closed after subject deletion")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled after subject deletion, got %v", g.modes.Active())
	}
}

// TestMoveMode_ExternalCancelClosesBreakConfirm: the break-confirm dialog
// must not be left stranded when move mode ends for a reason that has
// nothing to do with the dialog's own Confirm/Cancel buttons — e.g. an
// import wiping the mode via g.modes.Cancel("import") (import.go, reachable
// from the JS bridge) while the dialog is open. breakConfirmModeObserver
// (grid_move_break_confirm.go) exists specifically to close the dialog on
// any such external mode-cancel. RED before that observer is registered:
// breakConfirmActive() stays true and the Modal+Scrim portal is stranded.
func TestMoveMode_ExternalCancelClosesBreakConfirm(t *testing.T) {
	g, a, n, b, c, x, y := buildBreakConfirmSquareLayout(t)
	nID, aID, bID, cID, xID, yID := n.ID, a.ID, b.ID, c.ID, x.ID, y.ID

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected break-confirm dialog open")
	}

	// External cancel path: nothing to do with the dialog's own buttons.
	g.modes.Cancel("import")

	if g.breakConfirmActive() {
		t.Fatal("expected break-confirm dialog closed after external mode-cancel")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled, got %v", g.modes.Active())
	}
	moved := g.nodeAt(0, 6)
	if moved == nil || moved.ID != nID {
		t.Fatal("expected N to remain at (0,6) — graph untouched by external cancel")
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) ||
		!hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("expected all 4 square edges intact after external cancel")
	}
	if !hasDirectedEdge(g, xID, yID) {
		t.Fatal("expected the X-Y target edge intact (unsplit) after external cancel")
	}
}

// TestMoveMode_BreakConfirmStaleOccupiedDestination: the dialog's captured
// destination (i,j) can go stale while the dialog is open — a node placed at
// (i,j) after the dialog opens but before Confirm. ConfirmBreakCircuit must
// hit moveNode's occupied-destination guard and cancel the mode with reason
// "move-failed" rather than clobber the newly-placed node. Pins the guard
// documented in grid_move_break_confirm.go's ConfirmBreakCircuit.
func TestMoveMode_BreakConfirmStaleOccupiedDestination(t *testing.T) {
	canceled := make(chan hooks.GridModePayload, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventGridModeCanceled, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.GridModePayload); ok {
			canceled <- p
		}
	})
	defer u()

	g, a, n, b, c, _, _ := buildBreakConfirmSquareLayout(t)
	nID, aID, bID, cID := n.ID, a.ID, b.ID, c.ID

	g.modes.Enter(GridModeMove, n)
	tx, ty := screenPosForGridIJ(g, 10, 4)
	g.handleTapInGrid(tx, ty)
	if !g.breakConfirmActive() {
		t.Fatal("setup: expected break-confirm dialog open")
	}

	// Destination (10,4) goes stale: a node lands there before Confirm.
	g.tryAddNode(10, 4, model.NodeTypeRegular)

	g.ConfirmBreakCircuit()

	if g.breakConfirmActive() {
		t.Fatal("expected dialog closed after confirm attempt")
	}
	if g.modes.Active() != GridModeNone {
		t.Fatalf("expected mode canceled (move-failed), got %v", g.modes.Active())
	}
	moved := g.nodeAt(0, 6)
	if moved == nil || moved.ID != nID {
		t.Fatal("expected N to remain at (0,6) — corner node must not move onto the stale occupied destination")
	}
	if !hasDirectedEdge(g, aID, nID) || !hasDirectedEdge(g, nID, bID) ||
		!hasDirectedEdge(g, bID, cID) || !hasDirectedEdge(g, cID, aID) {
		t.Fatal("expected the square untouched")
	}

	select {
	case p := <-canceled:
		if p.Reason != "move-failed" {
			t.Fatalf("EventGridModeCanceled Reason = %q, want move-failed", p.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventGridModeCanceled after stale-occupied-destination confirm")
	}
}
