package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Meter bridge colors.
var (
	meterGreen  = color.RGBA{90, 180, 90, 255}
	meterYellow = color.RGBA{200, 200, 60, 255}
	meterRed    = color.RGBA{220, 60, 60, 255}
	meterBg     = color.RGBA{34, 34, 50, 255}
	meterClip   = color.RGBA{255, 40, 40, 255}
)

const (
	meterLabelW     = 50 // px reserved for instrument name
	meterClipW      = 20 // px reserved for clip indicator
	meterClipBoxW   = 12 // px for the actual clip box
	meterMinRowH    = 8
	meterMaxRowH    = 24
	meterDBFloor    = -60.0
	meterDBCeil     = 0.0
	meterYellowDB   = -6.0
	meterRedDB      = -1.0
	meterRMSOpacity = 128 // 50% of 255
	meterHeaderH    = 12  // px reserved for dB scale header
)

// drawMeterBridge draws a vertical stack of horizontal meter bars for all
// active instruments plus a master meter at the bottom.
func drawMeterBridge(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	// Always draw border.
	drawRect(dst, rect, colButtonBorder, false)

	if state == nil {
		return
	}

	// Collect active instruments.
	var active []analyzer.InstrumentMetrics
	for _, inst := range state.Instruments {
		if inst.ID != "" {
			active = append(active, inst)
		}
	}

	// Calculate row height: divide available height among instruments + master.
	numRows := len(active) + 1 // +1 for master
	availH := rect.Dy() - 2 - meterHeaderH
	rowH := availH / numRows
	if rowH < meterMinRowH {
		rowH = meterMinRowH
	}
	if rowH > meterMaxRowH {
		rowH = meterMaxRowH
	}

	x := rect.Min.X + 1 // inside border
	w := rect.Dx() - 2  // inside border

	// Draw dB scale header.
	scaleY := rect.Min.Y + 1
	barStartX := x + meterLabelW
	barEndX := x + w - meterClipW
	barW := barEndX - barStartX
	if barW > 0 {
		dbTicks := []float64{-48, -24, -12, -6, 0}
		captionScale := FontSizeCaption / FontSizeBody
		for _, db := range dbTicks {
			frac := dbToFrac(db)
			tickX := barStartX + int(frac*float64(barW))
			// Vertical reference line through all rows.
			drawRect(dst, image.Rect(tickX, scaleY+meterHeaderH, tickX+1, rect.Max.Y-1), color.NRGBA{255, 255, 255, 20}, true)
			// Label.
			label := fmt.Sprintf("%.0f", db)
			lw := int(float64(TextWidth(label)) * captionScale)
			DrawTextColorAtScale(dst, label, tickX-lw/2, scaleY, colTextSecondary, captionScale)
		}
	}

	y := rect.Min.Y + 1 + meterHeaderH

	// Draw each instrument meter.
	for _, inst := range active {
		drawMeterRow(dst, x, y, w, rowH, meterLabelW, inst.Name, inst.PeakDB, inst.RMSDB, inst.ClipCount)
		y += rowH
	}

	// Separator line before master.
	drawRect(dst, image.Rect(x, y, x+w, y+1), colWaveMid, true)
	y++

	// Master meter.
	masterH := rowH
	if remaining := rect.Max.Y - 1 - y; remaining < masterH {
		masterH = remaining
	}
	if masterH > 0 {
		drawMeterRow(dst, x, y, w, masterH, meterLabelW, "Master", state.Master.PeakDB, state.Master.RMSDB, state.Master.ClipCount)
	}
}

// drawMeterRow draws a single horizontal meter bar with peak, RMS overlay,
// and clip indicator.
func drawMeterRow(dst *ebiten.Image, x, y, width, height, labelW int, name string, peakDB, rmsDB float64, clips int) {
	// Background.
	drawRect(dst, image.Rect(x, y, x+width, y+height), meterBg, true)

	// Instrument name label.
	captionScale := FontSizeCaption / FontSizeBody
	truncated := truncateName(name, labelW-6, captionScale)
	textY := y + (height-int(float64(TextHeight())*captionScale))/2
	DrawTextColorAtScale(dst, truncated, x+4, textY, colTextSecondary, captionScale)

	// Bar area: after label, before clip indicator.
	barX := x + labelW
	barW := width - labelW - meterClipW
	if barW <= 0 {
		return
	}

	// Peak bar (full height).
	peakFrac := dbToFrac(peakDB)
	peakPx := int(peakFrac * float64(barW))
	if peakPx > 0 {
		peakCol := meterColor(peakDB)
		drawRect(dst, image.Rect(barX, y, barX+peakPx, y+height), peakCol, true)
	}

	// RMS overlay (half height, centered, 50% opacity).
	rmsFrac := dbToFrac(rmsDB)
	rmsPx := int(rmsFrac * float64(barW))
	if rmsPx > 0 {
		rmsH := height / 2
		if rmsH < 1 {
			rmsH = 1
		}
		rmsY := y + (height-rmsH)/2
		rmsCol := meterColorAlpha(rmsDB, meterRMSOpacity)
		drawRect(dst, image.Rect(barX, rmsY, barX+rmsPx, rmsY+rmsH), rmsCol, true)
	}

	// Clip indicator / peak dB readout.
	if clips > 0 {
		clipX := x + width - meterClipW + (meterClipW-meterClipBoxW)/2
		clipY := y + (height-meterClipBoxW)/2
		clipH := meterClipBoxW
		if clipH > height {
			clipH = height
			clipY = y
		}
		drawRect(dst, image.Rect(clipX, clipY, clipX+meterClipBoxW, clipY+clipH), meterClip, true)
	} else if peakDB > meterDBFloor {
		dbText := fmt.Sprintf("%.0f", peakDB)
		textCol := meterColor(peakDB)
		textX := x + width - meterClipW + 2
		dbTextY := y + (height-int(float64(TextHeight())*captionScale))/2
		DrawTextColorAtScale(dst, dbText, textX, dbTextY, textCol, captionScale)
	}
}

// dbToFrac converts a dB value to a [0,1] fraction, clamped.
func dbToFrac(db float64) float64 {
	if db <= meterDBFloor {
		return 0
	}
	if db >= meterDBCeil {
		return 1
	}
	return (db - meterDBFloor) / (meterDBCeil - meterDBFloor)
}

// meterColor returns the meter color for a given peak dB level.
func meterColor(db float64) color.RGBA {
	if db > meterRedDB {
		return meterRed
	}
	if db > meterYellowDB {
		return meterYellow
	}
	return meterGreen
}

// meterColorAlpha returns the meter color with a custom alpha for overlays.
func meterColorAlpha(db float64, alpha uint8) color.NRGBA {
	c := meterColor(db)
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: alpha}
}

// truncateName shortens name so it fits within maxPx at the given scale.
func truncateName(name string, maxPx int, scale float64) string {
	if int(float64(TextWidth(name))*scale) <= maxPx {
		return name
	}
	for i := len(name) - 1; i > 0; i-- {
		candidate := name[:i]
		if int(float64(TextWidth(candidate))*scale) <= maxPx {
			return candidate
		}
	}
	return name[:1]
}
