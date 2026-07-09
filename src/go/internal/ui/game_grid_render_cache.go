package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// gridRenderCache groups the top grid-pane's GPU-texture memoization: the grid
// tile/backing, the screen-space grid layer cache, the grid-pane SubImage
// wrapper, and the static node-layer / node-sprite / edge caches. Every field is
// owned by the Draw goroutine (no locks) and is pure render memoization keyed on
// camera/graph signatures — invalidated and rebuilt in the game_draw_* paths.
//
// It is embedded anonymously in Game so existing g.gridTileW / g.nodeLayer / …
// call sites keep working via field promotion; extracting it just lifts these
// ~34 caches out of the ~490-field Game struct into one cohesive, documented
// unit (deep-module grouping, no behaviour change).
//
// NOTE: the per-frame scratch gridDrawScreen / gridDrawCtx are intentionally NOT
// here — they stay on Game (pinned by grid_draw_dispatch_discipline_test.go), as
// does edgesDirty (a graph-edit dirty flag, not render memoization).
type gridRenderCache struct {
	// grid render cache
	gridTile       *ebiten.Image // SubImage view of gridTileBacking for the current stepPx
	gridTileStepPx int
	gridTileSubSig uint64
	// Grow-only backing texture for the grid tile. The logical tile is a
	// stepPx×stepPx square that changes size on every frame of a continuous
	// zoom; allocating it fresh each frame churns the GPU atlas and stalls the
	// single WASM thread (starving audio). Instead we keep one backing image
	// sized to the largest stepPx seen, redraw into its top-left region, and
	// hand out a SubImage — so an active zoom reuses the backing, allocating
	// only when a larger tile is needed. See buildGridTile.
	gridTileBacking     *ebiten.Image
	gridTileBackingSize int
	// Logical period (px) at which the caller tiles gridTile into the grid cache.
	// The tile is a multi-cell block (≥ gridTileMinBlockPx) so the per-rebuild
	// blit count stays ~area/gridTileW² instead of ~area/stepPx² — without the
	// block, tiling a tiny stepPx tile when zoomed out costs tens of thousands of
	// blits/frame and starves audio. See buildGridTile.
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

	// Cached SubImage wrapper for the grid pane region of the screen.
	// Reused across frames when the parent screen pointer and grid rect are
	// unchanged. Without this, drawGridPane's `screen.SubImage(...)` call
	// allocated a fresh *ebiten.Image wrapper every frame — at 60 FPS that
	// was the dominant non-zone allocation source behind a fast WASM OOM
	// (see playback_alloc_throughput_test.go).
	gridPaneSubParent *ebiten.Image
	gridPaneSubRect   image.Rectangle
	gridPaneSub       *ebiten.Image

	// Per-frame pre-computed node radii (avoids O(n²) neighbor checks in draw loop)
	nodeRadiiCache []float64

	// Reused scratch map of the open GroupMenu's member node ids, rebuilt
	// once per drawGridNodes call (not per node) and cleared-not-reallocated
	// across frames to keep member-ring rendering alloc-neutral.
	groupMenuMembersScratch map[model.NodeID]bool

	// Reused scratch map from NodeID to the ascending-GroupID list of every
	// group that node belongs to (per game_group_badges.go's multi-ring
	// rendering — up to 3 concentric rings per node, ordered by GroupID).
	// Rebuilt once per drawGridNodes call from g.graph.AllGroups() (already
	// GroupID-sorted) so the per-node ring loop never re-scans the group set.
	groupMembershipScratch map[model.NodeID][]model.GroupID

	// Static node layer cache: all nodes in default (non-highlighted) state.
	// Rebuilt only when camera moves beyond pad, graph changes, or scale changes.
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
