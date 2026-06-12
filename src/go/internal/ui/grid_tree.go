package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Grid-local z-index conventions for the GridTree component tree. These
// are independent of the drum-pane Z* constants in drumview_tree.go —
// the two trees own disjoint screen rects. Ascending z = drawn later =
// composites on top. The coordinate badge sits BELOW every popup/sidebar
// so it can never overpaint them (the bug this tree fixes structurally).
const (
	GZBackground  = 0
	GZEdges       = 10
	GZCanvas      = 20
	GZPulses      = 30
	GZCoordBadge  = 40
	GZMoveMode    = 42
	GZConnectMode = 44
	GZLongPress   = 50
	GZMoveConfirm = 52
	GZSidebar     = 60
	GZCursorLabel = 70
)

// GridTree orchestrates the grid pane's Zone/Layer tree. It mirrors
// DrumViewTree's 4-phase loop (Layout → Update → Input → Draw) and its
// capture/suppress lifecycle, but drops the drum-pane specifics
// (OverlayPortal, transport/eq keyboard auto-focus). It reuses the shared
// primitives Zone/Layer/HitArea/HitHandler/HitIndex/zoneAsLayer.
type GridTree struct {
	zones   []*gridZoneEntry
	zoneMap map[string]*gridZoneEntry
	layers  []Layer

	hitIndex *HitIndex

	capturedHandler HitHandler
	capturedTag     string
	suppress        bool
	wasPressed      bool
	inputHandled    bool
	wheelHandled    bool

	dragActive func() bool

	bounds image.Rectangle
}

type gridZoneEntry struct {
	zone        Zone
	rect        image.Rectangle
	zIndex      int
	lastRect    image.Rectangle
	visible     func() bool
	lastVisible bool
}

// NewGridTree creates a tree with a fresh HitIndex (no portal).
func NewGridTree() *GridTree {
	return &GridTree{
		zoneMap:  make(map[string]*gridZoneEntry),
		hitIndex: &HitIndex{},
	}
}

func (t *GridTree) RegisterZone(z Zone, zIndex int) { t.RegisterZoneVisible(z, zIndex, nil) }

func (t *GridTree) RegisterZoneVisible(z Zone, zIndex int, visible func() bool) {
	e := &gridZoneEntry{zone: z, zIndex: zIndex, visible: visible, lastVisible: true}
	t.zones = append(t.zones, e)
	t.zoneMap[z.ID()] = e
	t.insertLayer(zoneAsLayer{zone: z, zIndex: zIndex, visible: visible})
}

func (t *GridTree) RegisterLayer(l Layer) { t.insertLayer(l) }

func (t *GridTree) insertLayer(l Layer) {
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

// LayersForTest returns a snapshot of the merged draw slice in render order.
func (t *GridTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
}

func (t *GridTree) SetZoneRect(id string, r image.Rectangle) {
	if e, ok := t.zoneMap[id]; ok {
		e.rect = r
	}
}

func (t *GridTree) SetBounds(r image.Rectangle) { t.bounds = r }

func (t *GridTree) HitIndexRef() *HitIndex { return t.hitIndex }

func (t *GridTree) SetDragActive(fn func() bool) { t.dragActive = fn }

// Draw renders the merged slice in ascending z. Zones are clipped to the
// intersection of tree bounds and their rect; plain layers receive the
// unclipped screen and self-clip (mirrors DrumViewTree.Draw).
func (t *GridTree) Draw(screen *ebiten.Image) {
	for _, layer := range t.layers {
		if !layer.Visible() {
			continue
		}
		zl, isZone := layer.(zoneAsLayer)
		if !isZone {
			layer.Draw(screen)
			continue
		}
		clip := screen.Bounds()
		if !t.bounds.Empty() {
			clip = clip.Intersect(t.bounds)
		}
		if e, ok := t.zoneMap[zl.zone.ID()]; ok && !e.rect.Empty() {
			clip = clip.Intersect(e.rect)
		}
		if clip.Empty() {
			continue
		}
		if clip == screen.Bounds() {
			layer.Draw(screen)
		} else {
			layer.Draw(screen.SubImage(clip).(*ebiten.Image))
		}
	}
}
