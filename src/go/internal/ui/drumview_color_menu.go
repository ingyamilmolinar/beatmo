package ui

import (
	"image"
)

// buildColorMenu rebuilds the color picker dropdown for the selected row.
func (dv *DrumView) buildColorMenu() {
	dv.colorMenuBtns = dv.colorMenuBtns[:0]
	dv.colorHexBox = nil
	dv.colorWheelRect = image.Rect(0, 0, 0, 0)
	if dv.colorMenuRow < 0 || dv.colorMenuRow >= len(dv.rowColorBtns) {
		return
	}
	base := dv.rowColorBtns[dv.colorMenuRow].Rect()
	// Determine wheel size and placement; always keep fully within dv.Bounds.
	// Start from a target diameter based on row height, clamp to bounds.
	target := dv.rowHeight() * 6
	if target < 60 {
		target = 60
	}
	if target > 200 {
		target = 200
	}
	maxSize := dv.Bounds.Dx()
	if dv.Bounds.Dy() < maxSize {
		maxSize = dv.Bounds.Dy()
	}
	if maxSize < 1 {
		maxSize = 1
	}
	wheel := target
	if wheel > maxSize {
		wheel = maxSize
	}
	// If panel is extremely small, still render something.
	if wheel < 20 {
		wheel = maxSize
	}

	// Prefer opening upwards if there's room, else below; clamp both axes.
	wantY := base.Min.Y - wheel
	if wantY < dv.Bounds.Min.Y {
		// place below
		wantY = base.Max.Y
	}
	// Clamp to bounds
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}
	if wantY > dv.Bounds.Max.Y-wheel {
		wantY = dv.Bounds.Max.Y - wheel
	}
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}

	wantX := base.Min.X
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}
	if wantX > dv.Bounds.Max.X-wheel {
		wantX = dv.Bounds.Max.X - wheel
	}
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}

	dv.colorWheelRect = image.Rect(wantX, wantY, wantX+wheel, wantY+wheel)
	dv.logger.Debugf("[COLOR] build wheel: bounds=%v base=%v target=%d maxSize=%d final=%dx%d at=(%d,%d)", dv.Bounds, base, target, maxSize, dv.colorWheelRect.Dx(), dv.colorWheelRect.Dy(), dv.colorWheelRect.Min.X, dv.colorWheelRect.Min.Y)
	dv.rebuildColorWheelImage()
}
