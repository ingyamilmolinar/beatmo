package ui

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// rowSpriteCache holds the per-row cached sprites for the steps area (no
// highlights). Each sprite covers the full timeline width and one row height,
// rebuilt when length, row color, or timeline width/height change.
//
// It is embedded anonymously in DrumView so existing dv.rowCache /
// dv.rowCacheGen / … keep resolving via field promotion; grouping just names
// this cohesive cache and lifts its fields out of the ~590-field DrumView struct
// (no behaviour change). See drumview_cache_row_sprite.go.
type rowSpriteCache struct {
	rowCache        []*ebiten.Image
	rowDirty        []bool
	rowFullDirty    []bool
	rowCacheW       int
	rowCacheH       int
	rowCacheLen     int
	rowCacheOff     []int
	rowCacheGen     []int
	rowCacheSig     []uint64
	rowCacheSteps   [][]bool
	rowCacheTypes   [][]model.NodeType
	rowCachePadPx   int
	rowCacheShift   int
	rowCachePatch   int
	rowCacheFull    int
	rowCacheScratch []*ebiten.Image // double-buffer scratch for row sprite shifts
}
