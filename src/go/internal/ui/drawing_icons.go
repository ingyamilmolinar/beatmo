package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// All 29 IconID bodies, defined in the unified Lucide-style visual language.
//
// Each function takes a bounding rect; it composes itself in the 24-unit
// logical grid via icon_renderer.go's helpers and the iconCanvas mapping.
// Function-var pattern preserved so test overrides (icon_render_test.go)
// keep working.
//
// Visual primitives:
//   - Stroke weight: IconStrokeWeight (1.75 logical units, ≥1 px floor)
//   - Caps/joins: round
//   - Corner radius: IconCornerRadius (2 logical units) where applicable
//   - Antialiasing: always on

// ── Transport (4) ──────────────────────────────────────────────────────

var drawPlayIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillPath(dst, c, [][2]float32{{8, 5}, {19, 12}, {8, 19}}, col)
}

var drawPauseIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillRoundedRect(dst, c, 7, 5, 4, 14, 1.5, col)
	iconFillRoundedRect(dst, c, 13, 5, 4, 14, 1.5, col)
}

var drawStopIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillRoundedRect(dst, c, 6, 6, 12, 12, float32(IconCornerRadius), col)
}

var drawRecordIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeCircle(dst, c, 12, 12, 8, col)
	iconFillCircle(dst, c, 12, 12, 4, col)
}

// ── Editing / file (4) ──────────────────────────────────────────────────

var drawPencilIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Tip (5,19) → bottom-left guide, body shaft from bottom-left up to
	// upper-right tip, then a separate cross-stroke marks the bevel on
	// the eraser-end ferrule. Closed for a solid pencil silhouette.
	iconStrokePolyline(dst, c, [][2]float32{{5, 19}, {5, 15}, {15, 5}, {19, 9}, {9, 19}, {5, 19}}, col)
	iconStrokeLine(dst, c, 13, 7, 17, 11, col)
}

var drawSaveIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Floppy chassis (outer), top label slot, and inner name plate.
	iconStrokeOpenRoundedRect(dst, c, 4, 4, 16, 16, float32(IconCornerRadius), "", col)
	iconFillRoundedRect(dst, c, 8, 4, 8, 5, 0, col)
	iconStrokeOpenRoundedRect(dst, c, 8, 13, 8, 5, 0.5, "", col)
	iconFillCircle(dst, c, 14.5, 6.5, 0.6, col)
}

var drawCloseIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 6, 6, 18, 18, col)
	iconStrokeLine(dst, c, 18, 6, 6, 18, col)
}

var drawOverflowIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillCircle(dst, c, 12, 6, 1.5, col)
	iconFillCircle(dst, c, 12, 12, 1.5, col)
	iconFillCircle(dst, c, 12, 18, 1.5, col)
}

// ── Numeric / list (4) ──────────────────────────────────────────────────

var drawPlusIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 12, 5, 12, 19, col)
	iconStrokeLine(dst, c, 5, 12, 19, 12, col)
}

var drawMinusIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 5, 12, 19, 12, col)
}

var drawRowsIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 5, 8, 19, 8, col)
	iconStrokeLine(dst, c, 5, 12, 19, 12, col)
	iconStrokeLine(dst, c, 5, 16, 19, 16, col)
}

var drawAudioIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Five EQ bars with varying heights. x = 4, 7.5, 11, 14.5, 18 (3.5 spacing).
	heights := [5]float32{7, 12, 17, 10, 14}
	xs := [5]float32{4, 7.5, 11, 14.5, 18}
	for i := range heights {
		bottom := float32(20)
		top := bottom - heights[i]
		iconFillRoundedRect(dst, c, xs[i]-1.25, top, 2.5, heights[i], 0.5, col)
	}
}

// ── Chevrons (3) ───────────────────────────────────────────────────────

var drawChevronUpIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokePolyline(dst, c, [][2]float32{{6, 15}, {12, 9}, {18, 15}}, col)
}

var drawChevronDownIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokePolyline(dst, c, [][2]float32{{6, 9}, {12, 15}, {18, 9}}, col)
}

var drawChevronRightIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokePolyline(dst, c, [][2]float32{{9, 6}, {15, 12}, {9, 18}}, col)
}

var drawChevronLeftIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokePolyline(dst, c, [][2]float32{{15, 6}, {9, 12}, {15, 18}}, col)
}

// starPathPoints lists the 10 vertices of a 5-pointed star inscribed in
// the icon's 24×24 canvas. Outer radius 9, inner radius 3.5, centered
// at (12,12). Points alternate outer/inner clockwise from the top.
var starPathPoints = [][2]float32{
	{12.00, 3.00},   // top outer
	{14.06, 9.17},   // upper-right inner
	{20.56, 9.22},   // upper-right outer
	{15.33, 13.08},  // right inner
	{17.29, 19.28},  // lower-right outer
	{12.00, 15.50},  // bottom inner
	{6.71, 19.28},   // lower-left outer
	{8.67, 13.08},   // left inner
	{3.44, 9.22},    // upper-left outer
	{9.94, 9.17},    // upper-left inner
}

var drawStarIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	closed := append([][2]float32{}, starPathPoints...)
	closed = append(closed, starPathPoints[0])
	iconStrokePolyline(dst, c, closed, col)
}

var drawStarFilledIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillPath(dst, c, starPathPoints, col)
}

// ── Track / state (2) ──────────────────────────────────────────────────

var drawTrackIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Padlock body.
	iconStrokeOpenRoundedRect(dst, c, 5, 11, 14, 9, 1.5, "", col)
	// Closed shackle: arch from (8,11) up to (16,11), peaking near (12,5).
	iconStrokePolyline(dst, c, [][2]float32{{8, 11}, {8, 8}}, col)
	iconStrokeArc(dst, c, 12, 8, 4, float32(math.Pi), 2*float32(math.Pi), col)
	iconStrokePolyline(dst, c, [][2]float32{{16, 8}, {16, 11}}, col)
}

var drawTrackOffIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Open padlock — shackle's right arm raised away from the body.
	iconStrokeOpenRoundedRect(dst, c, 5, 11, 14, 9, 1.5, "", col)
	iconStrokePolyline(dst, c, [][2]float32{{8, 11}, {8, 8}}, col)
	iconStrokeArc(dst, c, 12, 8, 4, float32(math.Pi), 2*float32(math.Pi), col)
	// Right arm angles outward instead of returning to body.
	iconStrokeLine(dst, c, 16, 8, 19, 11, col)
}

// ── Upload / I/O (3) ──────────────────────────────────────────────────

var drawUploadIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Vertical shaft + upward arrow head.
	iconStrokeLine(dst, c, 12, 5, 12, 17, col)
	iconStrokePolyline(dst, c, [][2]float32{{7, 10}, {12, 5}, {17, 10}}, col)
	// Dashed top "destination" bar — three short segments.
	iconStrokeLine(dst, c, 4, 3, 8, 3, col)
	iconStrokeLine(dst, c, 10, 3, 14, 3, col)
	iconStrokeLine(dst, c, 16, 3, 20, 3, col)
}

var drawImportIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Open-top tray (no top edge).
	iconStrokeOpenRoundedRect(dst, c, 5, 15, 14, 5, 1.5, "top", col)
	// Vertical shaft pointing down + arrow head.
	iconStrokeLine(dst, c, 12, 4, 12, 14, col)
	iconStrokePolyline(dst, c, [][2]float32{{7, 9}, {12, 14}, {17, 9}}, col)
}

var drawExportIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Open-top tray (no top edge).
	iconStrokeOpenRoundedRect(dst, c, 5, 15, 14, 5, 1.5, "top", col)
	// Vertical shaft pointing up + arrow head.
	iconStrokeLine(dst, c, 12, 4, 12, 14, col)
	iconStrokePolyline(dst, c, [][2]float32{{7, 9}, {12, 4}, {17, 9}}, col)
}

// ── Audio (2) ──────────────────────────────────────────────────────────

func iconSpeakerCone(dst *ebiten.Image, c iconCanvas, col color.Color) {
	// Filled cone polygon — small back, expanding front.
	iconFillPath(dst, c, [][2]float32{
		{4, 9}, {8, 9}, {12, 5}, {12, 19}, {8, 15}, {4, 15},
	}, col)
}

var drawSpeakerIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconSpeakerCone(dst, c, col)
	// Two sound-wave arcs to the right of the cone.
	iconStrokeArc(dst, c, 14, 12, 3, -float32(math.Pi)/3, float32(math.Pi)/3, col)
	iconStrokeArc(dst, c, 16, 12, 5, -float32(math.Pi)/3, float32(math.Pi)/3, col)
}

var drawSpeakerOffIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconSpeakerCone(dst, c, col)
}

// ── Misc (5) ──────────────────────────────────────────────────────────

var drawNoteIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Note head — circle (close enough to ellipse at this scale).
	iconFillCircle(dst, c, 8.5, 17, 2.7, col)
	// Stem.
	iconStrokeLine(dst, c, 11, 17, 11, 5, col)
	// Flag (filled small wedge).
	iconFillPath(dst, c, [][2]float32{{11, 5}, {17, 7}, {17, 9}, {11, 8}}, col)
}

var drawMuteIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconSpeakerCone(dst, c, col)
	// Diagonal slash through the cone — round caps + the icon-stroke
	// width make this read as a unified "muted" mark.
	iconStrokeLine(dst, c, 4, 4, 20, 20, col)
}

var drawSoloIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Headphone band — half-circle arc from (4,12) to (20,12), peaking at (12,4).
	iconStrokeArc(dst, c, 12, 12, 8, float32(math.Pi), 2*float32(math.Pi), col)
	// Ear cups — two filled rounded rects.
	iconFillRoundedRect(dst, c, 3, 12, 5, 7, 1.5, col)
	iconFillRoundedRect(dst, c, 16, 12, 5, 7, 1.5, col)
}

var drawFxIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Sparkle: vertical + horizontal stroke through center, plus accent dots.
	iconStrokeLine(dst, c, 12, 5, 12, 15, col)
	iconStrokeLine(dst, c, 7, 10, 17, 10, col)
	iconFillCircle(dst, c, 18, 6, 0.9, col)
	iconFillCircle(dst, c, 5, 18, 0.9, col)
}

var drawTargetIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeCircle(dst, c, 12, 12, 6, col)
	iconStrokeLine(dst, c, 12, 4, 12, 9, col)
	iconStrokeLine(dst, c, 12, 15, 12, 20, col)
	iconStrokeLine(dst, c, 4, 12, 9, 12, col)
	iconStrokeLine(dst, c, 15, 12, 20, 12, col)
}

var drawTrashIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Bin body without top edge (lid sits on top).
	iconStrokeOpenRoundedRect(dst, c, 6, 7, 12, 13, 1.5, "top", col)
	// Lid bar across the top.
	iconStrokeLine(dst, c, 4, 7, 20, 7, col)
	// Lid handle (small filled tab).
	iconFillRoundedRect(dst, c, 10, 4, 4, 2, 1, col)
	// Two interior tick marks.
	iconStrokeLine(dst, c, 10, 11, 10, 17, col)
	iconStrokeLine(dst, c, 14, 11, 14, 17, col)
}

var drawCircleIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillCircle(dst, c, 12, 12, 8, col)
}

// Chevron + stem — a downward chevron with a vertical stem dropping into the
// trace area. Used on the Chain tab to mark the trigger sample at Spacious.
var drawTriggerMarkerIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 6, 6, 12, 12, col)
	iconStrokeLine(dst, c, 18, 6, 12, 12, col)
	iconStrokeLine(dst, c, 12, 12, 12, 20, col)
}

// Headroom — a horizontal ceiling bar with a downward arrow indicating the
// space below the limit. Used in the Levels icon-row cascade.
var drawHeadroomIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 4, 5, 20, 5, col)
	iconStrokeLine(dst, c, 12, 8, 12, 20, col)
	iconStrokeLine(dst, c, 8, 16, 12, 20, col)
	iconStrokeLine(dst, c, 16, 16, 12, 20, col)
}

// Clip count — triangle with an exclamation bang. Signals clipping events.
var drawClipCountIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeLine(dst, c, 12, 4, 21, 20, col)
	iconStrokeLine(dst, c, 21, 20, 3, 20, col)
	iconStrokeLine(dst, c, 3, 20, 12, 4, col)
	iconStrokeLine(dst, c, 12, 10, 12, 15, col)
	iconFillCircle(dst, c, 12, 18, 1, col)
}

// Loudest — a peak indicator: rising bars topped by a horizontal peak-hold
// line. Used to mark the loudest channel readout in the icon-row cascade.
var drawLoudestIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillRoundedRect(dst, c, 5, 16, 3, 4, 0.5, col)
	iconFillRoundedRect(dst, c, 10, 12, 3, 8, 0.5, col)
	iconFillRoundedRect(dst, c, 15, 7, 3, 13, 0.5, col)
	iconStrokeLine(dst, c, 3, 5, 20, 5, col)
}
