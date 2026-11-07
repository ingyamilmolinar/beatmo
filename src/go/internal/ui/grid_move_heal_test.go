package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newHealTestGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	return g
}

func TestMoveHealPlan(t *testing.T) {
	tests := []struct {
		name string
		// wantFrom/wantTo only for healPassThrough cases.
		build     func(g *Game) (n, wantFrom, wantTo *uiNode)
		wantClass healClass
	}{
		{
			name: "isolated",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				n := g.tryAddNode(2, 2, model.NodeTypeRegular)
				return n, nil, nil
			},
			wantClass: healSilent,
		},
		{
			name: "endpoint-in",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				g.addEdge(a, n)
				return n, nil, nil
			},
			wantClass: healSilent,
		},
		{
			name: "endpoint-out",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				a := g.tryAddNode(0, 8, model.NodeTypeRegular)
				g.addEdge(n, a)
				return n, nil, nil
			},
			wantClass: healSilent,
		},
		{
			name: "straight chain vertical",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				b := g.tryAddNode(0, 8, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(n, b)
				return n, a, b
			},
			wantClass: healPassThrough,
		},
		{
			name: "straight chain horizontal reversed",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				b := g.tryAddNode(8, 0, model.NodeTypeRegular)
				n := g.tryAddNode(4, 0, model.NodeTypeRegular)
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				g.addEdge(b, n)
				g.addEdge(n, a)
				return n, b, a
			},
			wantClass: healPassThrough,
		},
		{
			// Classification only needs A and B orthogonal to each other. The diagonal A-N leg is injected into g.edges
			// directly: g.addEdge refuses diagonals, and moveHealPlan only reads the slice.
			name: "collinear-but-offset pass-through",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(4, 4, model.NodeTypeRegular)
				b := g.tryAddNode(0, 8, model.NodeTypeRegular)
				g.edges = append(g.edges, uiEdge{A: a, B: n, pulse: -1})
				g.edges = append(g.edges, uiEdge{A: n, B: b, pulse: -1})
				return n, a, b
			},
			wantClass: healPassThrough,
		},
		{
			name: "corner",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				b := g.tryAddNode(4, 4, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(n, b)
				return n, nil, nil
			},
			wantClass: healUnhealable,
		},
		{
			// Each edge into or out of N must be orthogonal, so B shares N's J.
			name: "junction 3 edges",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				b := g.tryAddNode(4, 4, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				c := g.tryAddNode(0, 8, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(b, n)
				g.addEdge(n, c)
				return n, nil, nil
			},
			wantClass: healUnhealable,
		},
		{
			name: "double-in",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				b := g.tryAddNode(4, 4, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(b, n)
				return n, nil, nil
			},
			wantClass: healUnhealable,
		},
		{
			name: "double-out",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				b := g.tryAddNode(4, 4, model.NodeTypeRegular)
				g.addEdge(n, a)
				g.addEdge(n, b)
				return n, nil, nil
			},
			wantClass: healUnhealable,
		},
		{
			name: "two-cycle",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(n, a)
				return n, nil, nil
			},
			wantClass: healUnhealable,
		},
		{
			// An imported self-loop counts as neither in nor out, so one real incoming edge makes an endpoint (healSilent).
			name: "self-loop plus one incoming",
			build: func(g *Game) (*uiNode, *uiNode, *uiNode) {
				a := g.tryAddNode(0, 0, model.NodeTypeRegular)
				n := g.tryAddNode(0, 4, model.NodeTypeRegular)
				g.addEdge(a, n)
				g.addEdge(n, n)
				return n, nil, nil
			},
			wantClass: healSilent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newHealTestGame(t)
			n, wantFrom, wantTo := tt.build(g)

			plan := g.moveHealPlan(n)

			if plan.class != tt.wantClass {
				t.Fatalf("class = %v, want %v", plan.class, tt.wantClass)
			}
			if tt.wantClass == healPassThrough {
				if plan.from == nil || plan.to == nil {
					t.Fatalf("expected from/to set, got from=%v to=%v", plan.from, plan.to)
				}
				if plan.from.ID != wantFrom.ID {
					t.Fatalf("from.ID = %d, want %d", plan.from.ID, wantFrom.ID)
				}
				if plan.to.ID != wantTo.ID {
					t.Fatalf("to.ID = %d, want %d", plan.to.ID, wantTo.ID)
				}
			} else {
				if plan.from != nil || plan.to != nil {
					t.Fatalf("expected nil from/to for class %v, got from=%v to=%v", tt.wantClass, plan.from, plan.to)
				}
			}
		})
	}
}

func TestNodesOnSpan(t *testing.T) {
	g := newHealTestGame(t)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	m := g.tryAddNode(0, 3, model.NodeTypeRegular)
	b := g.tryAddNode(0, 8, model.NodeTypeRegular)

	t.Run("chain span returns interior node", func(t *testing.T) {
		got := g.nodesOnSpan(a, b)
		if len(got) != 1 || got[0].ID != m.ID {
			t.Fatalf("nodesOnSpan(a,b) = %v, want [%v]", got, m.ID)
		}
	})

	t.Run("adjacent span returns empty", func(t *testing.T) {
		got := g.nodesOnSpan(a, m)
		if len(got) != 0 {
			t.Fatalf("nodesOnSpan(a,m) = %v, want empty", got)
		}
	})

	t.Run("non-orthogonal pair returns empty", func(t *testing.T) {
		c := g.tryAddNode(5, 5, model.NodeTypeRegular)
		got := g.nodesOnSpan(a, c)
		if len(got) != 0 {
			t.Fatalf("nodesOnSpan(a,c) = %v, want empty", got)
		}
	})
}
