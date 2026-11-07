package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Draw-goroutine-only render memoization (no locks), embedded in Game so fields resolve by promotion.
// gridDrawScreen/gridDrawCtx and edgesDirty stay on Game.
type gridRenderCache struct {
	// grid render cache
	gridTile       *ebiten.Image // SubImage view of gridTileBacking for the current stepPx
	gridTileStepPx int
	gridTileSubSig uint64
	// gridTileThemeGen is the ThemeGeneration the tile was baked under.
	gridTileThemeGen int
	// Grow-only: the tile changes size every frame of a zoom, and reallocating churns the GPU atlas.
	gridTileBacking     *ebiten.Image
	gridTileBackingSize int
	// A multi-cell block period, so a rebuild costs ~area/gridTileW² blits, not ~area/stepPx².
	gridTileW int
	// grid layer cache (screen-space)
	gridCache       *ebiten.Image
	gridCacheW      int
	gridCacheH      int
	gridCacheScale  float64
	gridCacheOffX   float64
	gridCacheOffY   float64
	gridCachePad    int
	gridCacheStepPx int
	gridCacheSubSig uint64
	// The pad-shift reuse path must not survive a theme change.
	gridCacheThemeGen int

	// Reused while the parent and rect are unchanged: a SubImage per frame drove a WASM OOM.
	gridPaneSubParent *ebiten.Image
	gridPaneSubRect   image.Rectangle
	gridPaneSub       *ebiten.Image

	// Per-frame pre-computed node radii (avoids O(n²) neighbor checks in draw loop)
	nodeRadiiCache []float64

	// Cleared, not reallocated, so member rings stay allocation-free.
	groupMenuMembersScratch map[model.NodeID]bool

	// Rebuilt once per drawGridNodes call, so the per-node ring loop never rescans the groups.
	groupMembershipScratch map[model.NodeID][]model.GroupID

	// Rebuilt only when the camera leaves the pad, the graph changes or the scale changes.
	nodeLayer         *ebiten.Image
	nodeLayerDirty    bool
	nodeLayerCamOffX  float64
	nodeLayerCamOffY  float64
	nodeLayerCamScale float64
	nodeLayerGraphSig uint64 // hash of node positions + colors + types
	nodeLayerPad      int    // reuse tolerance for camera pans

	// node sprite cache (screen-space) keyed by radius px + colors
	nodeSpriteCache map[spriteKey]*ebiten.Image

	// edge static cache (screen-space)
	edgeCache         *ebiten.Image
	edgeCacheW        int
	edgeCacheH        int
	edgeCacheScale    float64
	edgeCacheOffX     float64
	edgeCacheOffY     float64
	edgeCacheCount    int
	edgeCachePad      int
	edgeCacheColorSig uint64
}
