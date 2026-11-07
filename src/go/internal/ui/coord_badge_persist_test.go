//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The badge never auto-hides on a timer: it stays while the node is selected and its sidebar open, and goes when
// another node is selected or the sidebar closes. Draw clears coordBadgeNode when it hides the badge, so one draw
// shows the decision.

// As the desktop click on an existing node does: select, open the sidebar, arm the badge.
func armCoordBadge(g *Game, n *uiNode) {
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
}

// Past the old 180-frame timer, while selected with the menu open.
func TestCoordBadgePersistsPastTimerWhileMenuOpen(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if Profile().IsMobile() {
		t.Fatalf("expected desktop profile after Layout(1280,720); IsMobile=true")
	}

	n := g.tryAddNode(3, 5, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	armCoordBadge(g, n)

	// Advance well past the old 180-frame (~3s) auto-hide window.
	g.frame = g.coordBadgeFrame + 600

	img := ebiten.NewImage(1280, 720)
	g.drawGridCoordBadge(img)

	if g.coordBadgeNode != n {
		t.Fatalf("badge disappeared after %d frames while node stayed selected and menu open; "+
			"coordBadgeNode=%v want %v", g.frame-g.coordBadgeFrame, g.coordBadgeNode, n)
	}
}

func TestCoordBadgeHidesWhenMenuClosed(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	n := g.tryAddNode(3, 5, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	armCoordBadge(g, n)

	g.sidebar.Close()
	g.frame = g.coordBadgeFrame + 1

	img := ebiten.NewImage(1280, 720)
	g.drawGridCoordBadge(img)

	if g.coordBadgeNode != nil {
		t.Fatalf("badge should be hidden after the pop-up menu was closed; coordBadgeNode=%v", g.coordBadgeNode)
	}
}

func TestCoordBadgeHidesWhenDifferentNodeSelected(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	n1 := g.tryAddNode(3, 5, model.NodeTypeRegular)
	if n1 == nil {
		t.Fatal("tryAddNode(n1) returned nil")
	}
	// tryAddNode arms the badge for its node, so n2 is made first and the badge then armed for n1.
	n2 := g.tryAddNode(6, 2, model.NodeTypeRegular)
	if n2 == nil {
		t.Fatal("tryAddNode(n2) returned nil")
	}
	armCoordBadge(g, n1)

	// The draw-time guard must hide the badge still pointing at n1.
	g.sel = n2
	n2.Selected = true
	g.sidebar.Open(n2)
	g.frame = g.coordBadgeFrame + 1

	img := ebiten.NewImage(1280, 720)
	g.drawGridCoordBadge(img)

	if g.coordBadgeNode == n1 {
		t.Fatalf("old node's badge should be hidden after a different node was selected; coordBadgeNode still=%v", n1)
	}
}
