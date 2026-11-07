package ui

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Embedded in DrumView, so its fields resolve by promotion.
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
