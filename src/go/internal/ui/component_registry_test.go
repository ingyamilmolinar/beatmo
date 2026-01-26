package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

type spyComponent struct {
	id             string
	initCount      int
	graphChanges   int
	renderPhases   map[RenderPhase]int
	lastCtx        ComponentContext
	simpleDrawHits int
}

func (s *spyComponent) ID() string { return s.id }

func (s *spyComponent) Init(ctx ComponentContext) {
	s.initCount++
	s.lastCtx = ctx
}

func (s *spyComponent) OnGraphChange(ctx ComponentContext, change GraphChange) {
	s.graphChanges++
	s.lastCtx = ctx
}

func (s *spyComponent) Render(ctx ComponentContext, phase RenderPhase, _ *ebiten.Image) {
	if s.renderPhases == nil {
		s.renderPhases = make(map[RenderPhase]int)
	}
	s.renderPhases[phase]++
	if ctx.Drum != nil && ctx.Drum.simpleDraw {
		s.simpleDrawHits++
	}
	s.lastCtx = ctx
}

func TestComponentRegistryRegistersAfterInit(t *testing.T) {
	assertDefaultParityState(t)
	reg := NewComponentRegistry()
	ctx := ComponentContext{}

	first := &spyComponent{id: "first"}
	second := &spyComponent{id: "second"}

	reg.Register(first)
	reg.Init(ctx)
	if first.initCount != 1 {
		t.Fatalf("first initCount=%d want 1", first.initCount)
	}
	if second.initCount != 0 {
		t.Fatalf("second init before register, got %d", second.initCount)
	}

	reg.Register(second)
	if second.initCount != 1 {
		t.Fatalf("second initCount=%d want 1 after post-init Register", second.initCount)
	}

	reg.NotifyGraphChange(GraphChange{PathsChanged: true})
	if first.graphChanges != 1 || second.graphChanges != 1 {
		t.Fatalf("graph changes not delivered: first=%d second=%d", first.graphChanges, second.graphChanges)
	}
}

func TestGameComponentRegistryIntegratesWithDrumView(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	assertDefaultSimpleDraw(t, g)

	reg := g.ensureComponentRegistry()
	spy := &spyComponent{id: "spy"}
	reg.Register(spy)

	if spy.initCount != 1 {
		t.Fatalf("spy initCount=%d want 1", spy.initCount)
	}
	if spy.lastCtx.Drum != g.drum {
		t.Fatalf("component received unexpected drum context")
	}

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.pendingStartRow = -1
	g.start = a
	g.graph.StartNodeID = a.ID

	// Baseline compute.
	if err := g.Update(); err != nil {
		t.Fatalf("baseline update: %v", err)
	}
	baseChanges := spy.graphChanges

	// Modify graph to trigger path change.
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(b, c)
	g.graph.Edges[[2]model.NodeID{c.ID, a.ID}] = struct{}{}

	if err := g.Update(); err != nil {
		t.Fatalf("update after change: %v", err)
	}
	if spy.graphChanges <= baseChanges {
		t.Fatalf("expected graph change notification, got %d -> %d", baseChanges, spy.graphChanges)
	}

	// Ensure render hook executes during draw.
	dst := ebiten.NewImage(1280, 720)
	g.simpleDraw = false
	g.drawDrumPane(dst)
	if len(spy.renderPhases) == 0 {
		t.Fatalf("expected render hooks to fire, none recorded")
	}

	// Simple draw mode should still invoke component but mark simpleDrawHits.
	g.simpleDraw = true
	g.drawDrumPane(dst)
	if spy.simpleDrawHits == 0 {
		t.Fatalf("expected simple draw render to be observed")
	}
}

func TestDrumViewBuiltinFallbackWithoutRegistry(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)
	if dv.simpleDraw {
		t.Fatalf("simpleDraw=%v want false by default in DrumView", dv.simpleDraw)
	}
	dv.simpleDraw = false

	dst := ebiten.NewImage(800, 400)
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	// No assertion other than ensuring no panic; Draw should succeed with builtin phases.
}
