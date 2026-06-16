package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Grid-local z-index conventions for the GridTree draw stack. These are
// independent of the drum-pane Z* constants in drumview_tree.go — the two
// trees own disjoint screen rects. Ascending z = drawn later = composites on
// top. The coordinate badge sits BELOW every popup/sidebar so it can never
// overpaint them (the bug this stack fixes structurally).
const (
	GZBackground     = 0
	GZEdges          = 10
	GZCanvas         = 20
	GZPulses         = 30
	GZCoordBadge     = 40
	GZMoveMode       = 42
	GZConnectMode    = 44
	GZLongPress      = 50
	GZMoveConfirm    = 52
	GZSidebar        = 60
	GZCursorLabel    = 70
	GZGridHelpButton = 80
)

// GridTree is the grid pane's draw-ordered Layer stack — a slim sibling to
// DrumViewTree that owns ONLY z-ordered compositing for the top grid pane.
//
// It deliberately does NOT carry the drum pane's input machinery
// (HitIndex/Zone dispatch, capture/suppress lifecycle, OverlayPortal, keyboard
// focus). Grid pointer input still runs through the legacy path in
// Game.Update, which already has working z-prioritization (inputDispatcher +
// handleTapInGrid + cam.HandleMouse). The single job here is "draw the grid
// participants in ascending z so overlays composite OVER the badge" — the
// structural fix for the coordinate-badge-over-popup bug.
//
// If grid input is ever migrated into a tree (the deferred Phase 3 in
// docs/superpowers/plans/2026-06-12-grid-tree-zaxis.md), the Zone/HitIndex
// half is reintroduced deliberately — it is intentionally absent here rather
// than carried dormant, so there is no tested-but-unwired input code to rot.
type GridTree struct {
	layers []Layer

	bounds image.Rectangle

	// Cached sub-image for ClipToBounds layers — one shared wrapper for the
	// whole bounds rect, reused across frames to avoid per-frame SubImage
	// allocation (WASM-OOM concern). Mirrors the original drawGridPane's
	// cached `top` subimage.
	boundsSubParent *ebiten.Image
	boundsSubClip   image.Rectangle
	boundsSub       *ebiten.Image
}

// NewGridTree creates an empty draw stack.
func NewGridTree() *GridTree { return &GridTree{} }

// RegisterLayer inserts a draw-only Layer, keeping t.layers sorted ascending
// by ZIndex (stable for equal z by insertion order).
func (t *GridTree) RegisterLayer(l Layer) {
	z := l.ZIndex()
	idx := len(t.layers)
	for i, existing := range t.layers {
		if existing.ZIndex() > z {
			idx = i
			break
		}
	}
	t.layers = append(t.layers, nil)
	copy(t.layers[idx+1:], t.layers[idx:])
	t.layers[idx] = l
}

// LayersForTest returns a snapshot of the draw slice in render order.
// Production code MUST NOT call this — iterating the real slice from outside
// the tree breaks the render-pipeline encapsulation the discipline test enforces.
func (t *GridTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
}

func (t *GridTree) SetBounds(r image.Rectangle) { t.bounds = r }

// Draw renders the registered layers in ascending z. A layer that reports
// ClipToBounds()==true is drawn into a cached bounds-clipped subimage (== the
// original `top = screen.SubImage(gridRect)`); every other layer draws to the
// unclipped screen and self-clips (sidebar, cursor label).
func (t *GridTree) Draw(screen *ebiten.Image) {
	for _, layer := range t.layers {
		if !layer.Visible() {
			continue
		}
		if cl, ok := layer.(interface{ ClipToBounds() bool }); ok && cl.ClipToBounds() && !t.bounds.Empty() {
			clip := screen.Bounds().Intersect(t.bounds)
			if clip.Empty() {
				continue
			}
			if clip == screen.Bounds() {
				layer.Draw(screen)
			} else {
				layer.Draw(t.boundsSubImage(screen, clip))
			}
			continue
		}
		layer.Draw(screen)
	}
}

// boundsSubImage returns a cached SubImage of screen clipped to clip, reused
// while (screen, clip) are unchanged. Shared by all ClipToBounds layers so the
// whole grid-content set allocates at most one wrapper/frame.
func (t *GridTree) boundsSubImage(screen *ebiten.Image, clip image.Rectangle) *ebiten.Image {
	if t.boundsSubParent == screen && t.boundsSubClip == clip && t.boundsSub != nil {
		return t.boundsSub
	}
	sub := screen.SubImage(clip).(*ebiten.Image)
	t.boundsSubParent = screen
	t.boundsSubClip = clip
	t.boundsSub = sub
	return sub
}
