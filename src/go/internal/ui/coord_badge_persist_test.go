//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests pin the reported behavior: the (i,j) coordinate badge that
// appears next to a node when it is clicked must NOT auto-hide on a timer.
// It stays visible while the node stays selected and its pop-up menu (the
// node sidebar) is open, and only disappears when the user (a) selects a
// different node, or (b) closes the pop-up menu.
//
// drawGridCoordBadge self-clears g.coordBadgeNode = nil when it decides the
// badge should be hidden, so we can observe the visibility decision by
// drawing once and checking whether g.coordBadgeNode survives.

// armCoordBadge mirrors the real desktop click-on-existing-node path
// (game_input_editor.go): select the node, open its sidebar menu, and arm the
// coordinate badge for it.
func armCoordBadge(g *Game, n *uiNode) {
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
}

// TestCoordBadgePersistsPastTimerWhileMenuOpen is the failing repro: on
// desktop the badge used to vanish after 180 frames (~3s). It must stay
// visible while the node is selected and its menu is open.
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

// TestCoordBadgeHidesWhenMenuClosed pins hide-trigger (b): closing the pop-up
// menu (the node sidebar) hides the badge.
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

// TestCoordBadgeHidesWhenDifferentNodeSelected pins hide-trigger (a):
// selecting a different node hides the old node's badge.
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
	// Create n2 first (tryAddNode arms the badge for the node it creates), then
	// arm the badge for n1 so we can drive the draw-time guard directly.
	n2 := g.tryAddNode(6, 2, model.NodeTypeRegular)
	if n2 == nil {
		t.Fatal("tryAddNode(n2) returned nil")
	}
	armCoordBadge(g, n1)

	// A different node is now the selection with its menu open, while the badge
	// still points at n1 (the draw-time guard must catch this and hide it).
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
