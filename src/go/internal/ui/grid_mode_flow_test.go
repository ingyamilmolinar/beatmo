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

// Driven through handleTapInGrid (touch), never by poking mode fields. Layout: A(0,0)──B(0,20) vertical; M(10,0)
// moves, with no edges; (50,50) is a free cell and B an occupied endpoint.

// A tap on an integer coordinate then resolves by exact coincidence only, never a proximity snap, whatever
// the default camera scale.
func shrinkGuideSnapRadius(g *Game) {
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.1 * u)
}

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

// M3→M2 in and M2→D out form a corner, so M2 is healUnhealable; a direct moveNode still breaks silently (no heal
// edge, no panic), and the A-B splice is unaffected.
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

	// Direct, bypassing the interactive confirm dialog.
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

// A(0,0)→N(0,4)→B(0,8), N moved to (0,6) on its own N-B edge: the heal re-splices at N's new position, never
// leaving an unsplit A→B, and one undo restores the chain.
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

// M has its own pass-through P→M→Q too, so one move both heals (P-Q) and splices (A-B); one undo reverts all of
// it.
func TestMoveMode_SingleUndoStep(t *testing.T) {
	g, a, b, m := buildMoveModeLayout(t)

	// Orthogonal on the I=10 column, distinct from the A-B edge M splices into.
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

// These drive the real drawGridPane and intercept drawRect: screen.At() panics under real Ebiten.

type moveModeRectCall struct {
	Rect   image.Rectangle
	Color  color.RGBA
	Filled bool
}

// Keeps the outline-only calls, which collectFilledRects discards.
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

// drawRect blits one filled rect per call, so a matching call's intersected area is exactly its coverage.
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

// Blink phase 0 is ON and past gridModeBlinkHalfPeriod OFF: the subject's rect gets strictly more glow ON.
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

// A valid target (an A-B edge interior) draws a filled glow ghost; an invalid free cell draws none, only an
// outline ghost.
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

	// Centred on the pane: at shrinkGuideSnapRadius's huge scale a raw screen point leaves the window, and the ghost
	// would be skipped for the wrong reason.
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

// popup.opened / popup.closed / ui.tap / mode.armed, for both Move activation and an outside-release dismiss.
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

// The clipboard holds the node's type and params, copy mode is armed, and both events fire.
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

// M carries a distinctive Volume so paste-preserves-params is checkable; B is the occupied target.
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

// The paste has M's type and params, M is unchanged, the mode commits, and exactly one node is added.
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

// Stitched like a manual add: A-B becomes A-paste and paste-B.
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

// The overwrite confirm owns the flow from there, and the armed mode has committed.
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

// Through handleEscape, the single Esc authority.
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

// Desktop mouse path through g.Update, one frame per call; touchOverrideActive stays false.

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

// The hold consumes the press, so the release does not also open the sidebar.
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

	// The popup owns input, so the release never reaches handleEditor; a closed sidebar proves it.
	setLeft(false)
	_ = g.Update()

	if g.sidebar.IsOpen() {
		t.Fatal("sidebar should not open after a mouse-hold that opened the popup")
	}
}

// Moving past tapMaxMovePx kills the hold for that press, however long the button stays down.
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

	setPos(cx+tapMaxMovePx+20, cy)
	for i := 0; i < mouseHoldPopupFrames+20; i++ {
		_ = g.Update()
	}

	if g.longPressPopup {
		t.Fatal("popup should never open once the hold was canceled by drag")
	}

	// A test that ends holding leaks camera.go's package-level lastMouse, so the next test's first press reads as
	// a drag.
	setLeft(false)
	_ = g.Update()
}

// A quick press+release under the hold threshold still opens the sidebar.
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

// Slide into Move and release, as on touch.
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

	// The popup intercept handles hover tracking the same way for mouse and touch.
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

// The hold detector needs a node under the press.
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

	// Ending held would leak camera.go's lastMouse into the next test.
	setLeft(false)
	_ = g.Update()
}

// Deleting the armed node must not leave the mode manager on a dangling *uiNode; the prune runs once per Update
// and on Import.
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

// Square A(0,0)→N(0,6)→B(6,6)→C(6,0)→A, plus a target edge X(10,0)→Y(10,8) whose (10,4) is a valid GuideEdge.

// N is the corner: in from A (vertical), out to B (horizontal), so healUnhealable.
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

// Three edges: only exactly 1 in + 1 out can pass through.
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

// Two in, none out: not a pass-through.
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

// Two out, none in: not a pass-through.
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

// The dialog opens, the graph is unchanged and the mode stays armed on N.
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

// N splices into X-Y, A→N and N→B go with no heal A→B, and one undo restores the square and unsplits X-Y.
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

// Cancel leaves the graph untouched and the mode armed, so a re-tap retries; the first Esc closes the dialog,
// the second disarms.
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

// Cancel emits popup.closed and no mode.canceled; a re-tap + Confirm emits node.moved.
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

// Deleting the armed subject closes the dialog too, so no dangling breakPending.node lingers.
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

// A mode ended from outside (an import's Cancel, reachable from the JS bridge) must close the dialog, or its
// modal scrim is stranded.
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

// A node placed at the captured destination while the dialog is open: Confirm cancels with "move-failed"
// instead of clobbering it.
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
