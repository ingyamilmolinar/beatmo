package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Independent of the drum-pane Z constants (the trees own disjoint rects). Higher z composites on
// top; the coordinate badge sits below every popup so it can never overpaint them.
const (
	GZBackground     = 0
	GZEdges          = 10
	GZCanvas         = 20
	GZPulses         = 30
	GZGuide          = 35
	GZCoordBadge     = 40
	GZGroupBadges    = 41
	GZMoveMode       = 42
	GZConnectMode    = 44
	GZModeBanner     = 45
	GZMarquee        = 46
	GZLongPress      = 50
	GZSidebar        = 60
	GZGroupMenu      = 62
	GZCursorLabel    = 70
	GZToast          = 75
	GZGridHelpButton = 80
)

// Draw ordering only, by design: grid input keeps its own path in Game.Update, which already has
// working z-priority. A Zone/HitIndex half would be added deliberately, never carried dormant.
type GridTree struct {
	layers []Layer

	bounds image.Rectangle

	// One shared wrapper reused across frames: a SubImage per frame was a WASM OOM source.
	boundsSubParent *ebiten.Image
	boundsSubClip   image.Rectangle
	boundsSub       *ebiten.Image
}

// NewGridTree creates an empty draw stack.
func NewGridTree() *GridTree { return &GridTree{} }

// Stable for equal z.
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

// Production code must not iterate the real slice from outside the tree.
func (t *GridTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
}

func (t *GridTree) SetBounds(r image.Rectangle) { t.bounds = r }

// ClipToBounds layers draw into a cached clipped subimage; the rest get the unclipped screen and
// self-clip.
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

// At most one wrapper per frame for the whole grid-content set.
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
