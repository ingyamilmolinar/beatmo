package ui

import "github.com/hajimehoshi/ebiten/v2"

// Embedded in DrumView, so its fields resolve by promotion.
type rowsLayerCache struct {
	rowsLayer       *ebiten.Image
	rowsLayerW      int
	rowsLayerH      int
	rowsLayerOffset int
	rowsLayerRowOff int
	rowsLayerBaseX  int
	// Shift-and-fill only refills the right strip, so a width or length change without rowsLayerDirty
	// would leave the left pixels at the old cell pitch.
	rowsLayerRowWidth int
	rowsLayerLength   int
	rowsLayerGen      int
	// For the slim-bar diagnostic.
	lastRowsRenderPath    string
	lastLegacyRebuildKind string
	rowsLayerDirty        bool
	rowsLayerPadPx        int
	rowsLayerScratch      *ebiten.Image // double-buffer scratch for layer shifts
	rowsLayerBytes        int64
	rowsLayerFrame        int64
}
