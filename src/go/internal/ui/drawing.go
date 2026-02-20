package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawRect draws a rectangle. It is defined as a variable so tests can
// override it to capture draw calls.
var drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
	if r.Empty() {
		return
	}
	// Fast path: draw axis-aligned rectangles via scaled 1x1 pixel blits.
	// This avoids vector path overhead and is significantly faster in WASM.
	px := pixel(c)
	if filled {
		var op ebiten.DrawImageOptions
		op.GeoM.Scale(float64(r.Dx()), float64(r.Dy()))
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(px, &op)
		return
	}
	// 1px stroke on each side.
	// Top
	var top ebiten.DrawImageOptions
	top.GeoM.Scale(float64(r.Dx()), 1)
	top.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(px, &top)
	// Bottom
	var bot ebiten.DrawImageOptions
	bot.GeoM.Scale(float64(r.Dx()), 1)
	bot.GeoM.Translate(float64(r.Min.X), float64(r.Max.Y-1))
	dst.DrawImage(px, &bot)
	// Left
	var left ebiten.DrawImageOptions
	left.GeoM.Scale(1, float64(r.Dy()))
	left.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(px, &left)
	// Right
	var right ebiten.DrawImageOptions
	right.GeoM.Scale(1, float64(r.Dy()))
	right.GeoM.Translate(float64(r.Max.X-1), float64(r.Min.Y))
	dst.DrawImage(px, &right)
}

// drawButton renders a filled rectangle with a border. It can be overridden in tests.
var drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
	if r.Empty() {
		return
	}
	fc := fill
	if pressed {
		fc = adjustColor(fill, -20)
	}
	// Fill
	drawRect(dst, r, fc, true)
	// Subtle top-edge highlight for depth (skip on mobile for flat look).
	if !isSmallScreen() {
		highlight := adjustColor(fc, 10)
		drawRect(dst, image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+2), highlight, true)
	}
	// Border
	drawRect(dst, r, border, false)
}

// drawRoundedButton renders a filled rounded rectangle with a border.
// Same as drawButton but uses drawRoundedRect for fill and border.
var drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
	if r.Empty() {
		return
	}
	fc := fill
	if pressed {
		fc = adjustColor(fill, -20)
	}
	drawRoundedRect(dst, r, fc, radius, true)
	drawRoundedRect(dst, r, border, radius, false)
}

// popupButtonRadius returns the corner radius for popup action buttons.
func popupButtonRadius() int { return RadiusMD }

// adjustColor lightens or darkens a color by delta (-255..255).
func adjustColor(c color.Color, delta int) color.Color {
	r, g, b, a := c.RGBA()
	rr := clamp(int(r>>8)+delta, 0, 255)
	gg := clamp(int(g>>8)+delta, 0, 255)
	bb := clamp(int(b>>8)+delta, 0, 255)
	return color.RGBA{uint8(rr), uint8(gg), uint8(bb), uint8(a >> 8)}
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// Icon primitives (screen-space, font independent)
var drawPlayIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	// Right-pointing triangle with proportional padding and optical center shift.
	if r.Empty() {
		return
	}
	dim := minI(r.Dx(), r.Dy())
	pad := dim / 4 // 25% breathing room for small buttons
	if pad < 1 {
		pad = 1
	}
	inner := insetRect(r, pad)
	if inner.Empty() {
		return
	}
	// Optical center-of-mass shift: triangles appear left-heavy, nudge right ~8%.
	shift := dim / 12
	x0 := inner.Min.X + shift
	x1 := inner.Max.X
	y0 := inner.Min.Y
	y1 := inner.Max.Y - 1
	h := y1 - y0
	if h <= 0 {
		return
	}
	// Single continuous sweep from top to bottom (eliminates midline gap).
	for y := y0; y <= y1; y++ {
		// Distance from the vertical center, normalized to [0,1].
		mid := float64(y0+y1) / 2
		dist := math.Abs(float64(y) - mid)
		t := 1 - dist/float64(max1(h/2))
		if t < 0 {
			t = 0
		}
		xr := x0 + int(math.Round(t*float64(x1-x0)))
		if xr > x0 {
			drawRect(dst, image.Rect(x0, y, xr, y+1), col, true)
		}
	}
}

var drawPauseIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	w := r.Dx()
	gap := max1(w / 6)
	bar := max1(w / 5)
	left := image.Rect(r.Min.X, r.Min.Y, minI(r.Min.X+bar, r.Max.X), r.Max.Y)
	right := image.Rect(maxI(r.Max.X-bar, r.Min.X), r.Min.Y, r.Max.X, r.Max.Y)
	// Ensure spacing
	if right.Min.X-left.Max.X < gap {
		right.Min.X = left.Max.X + gap
		if right.Min.X > r.Max.X {
			right.Min.X = r.Max.X
		}
	}
	drawRect(dst, left, col, true)
	drawRect(dst, right, col, true)
}

var drawStopIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	dim := minI(r.Dx(), r.Dy())
	size := dim * 55 / 100 // 55% for refined proportion
	if size < 2 {
		size = dim
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	rect := image.Rect(cx-size/2, cy-size/2, cx+size/2, cy+size/2)
	radius := 1
	if size > 8 {
		radius = 2
	}
	drawRoundedRect(dst, rect, col, radius, true)
}

var drawPencilIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	// Draw a thin diagonal stroke from bottom-left toward top-right.
	w := r.Dx()
	h := r.Dy()
	thickness := max1(minI(w, h) / 10)
	if thickness < 2 {
		thickness = 2
	}
	x0 := r.Min.X + w/6
	y0 := r.Max.Y - h/6
	x1 := r.Max.X - w/6
	y1 := r.Min.Y + h/6
	steps := max1(absI(x1 - x0))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		drawRect(dst, image.Rect(x, y, x+thickness, y+thickness), col, true)
	}
}

var drawSaveIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	size := minI(r.Dx(), r.Dy())
	pad := max1(size / 6)
	body := image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
	if body.Empty() {
		return
	}
	drawRect(dst, body, col, false)
	labelH := max1(body.Dy() / 4)
	label := image.Rect(body.Min.X+1, body.Min.Y+1, body.Max.X-1, body.Min.Y+1+labelH)
	if label.Dx() > 0 && label.Dy() > 0 {
		drawRect(dst, label, col, true)
	}
	notchW := max1(body.Dx() / 4)
	notchH := max1(body.Dy() / 4)
	cx := (body.Min.X + body.Max.X) / 2
	notch := image.Rect(cx-notchW/2, body.Max.Y-notchH-1, cx+notchW/2, body.Max.Y-1)
	if notch.Dx() > 0 && notch.Dy() > 0 {
		drawRect(dst, notch, col, true)
	}
}

var drawCloseIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	// Draw an X using two diagonal strokes with 15% inset.
	dim := minI(r.Dx(), r.Dy())
	inset := dim * 15 / 100
	r = insetRect(r, inset)
	if r.Empty() {
		return
	}
	thickness := max1(minI(r.Dx(), r.Dy()) / 6)
	if thickness < 1 {
		thickness = 1
	}
	x0, y0 := r.Min.X, r.Min.Y
	x1, y1 := r.Max.X-1, r.Max.Y-1
	steps := max1(absI(x1 - x0))
	// Top-left to bottom-right
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		drawRect(dst, image.Rect(x, y, x+thickness, y+thickness), col, true)
	}
	// Top-right to bottom-left
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x1) - t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		drawRect(dst, image.Rect(x, y, x+thickness, y+thickness), col, true)
	}
}

// drawPlusIcon draws a geometric plus sign inside the bounds.
var drawPlusIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	thick := maxI(inner.Dx()/4, 2)
	cx := (inner.Min.X + inner.Max.X) / 2
	cy := (inner.Min.Y + inner.Max.Y) / 2
	// Horizontal bar
	drawRect(dst, image.Rect(inner.Min.X, cy-thick/2, inner.Max.X, cy-thick/2+thick), col, true)
	// Vertical bar
	drawRect(dst, image.Rect(cx-thick/2, inner.Min.Y, cx-thick/2+thick, inner.Max.Y), col, true)
}

// drawMinusIcon draws a geometric minus sign inside the bounds.
var drawMinusIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	thick := maxI(inner.Dx()/4, 2)
	cy := (inner.Min.Y + inner.Max.Y) / 2
	// Horizontal bar only
	drawRect(dst, image.Rect(inner.Min.X, cy-thick/2, inner.Max.X, cy-thick/2+thick), col, true)
}

// drawRoundedRect draws a filled or stroked rectangle with rounded corners.
// Uses a stepped approximation: the body is drawn inset by radius, then
// corner fills are added using overlapping rects. Radius is clamped to
// half the smallest dimension.
var drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
	if r.Empty() {
		return
	}
	maxR := minI(r.Dx(), r.Dy()) / 2
	if radius > maxR {
		radius = maxR
	}
	if radius < 1 {
		drawRect(dst, r, c, filled)
		return
	}
	if filled {
		// Center body (full width, excluding top/bottom corner rows)
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y+radius, r.Max.X, r.Max.Y-radius), c, true)
		// Top strip (inset by radius on each side)
		drawRect(dst, image.Rect(r.Min.X+radius, r.Min.Y, r.Max.X-radius, r.Min.Y+radius), c, true)
		// Bottom strip
		drawRect(dst, image.Rect(r.Min.X+radius, r.Max.Y-radius, r.Max.X-radius, r.Max.Y), c, true)
		// Corner fills: stepped rects approaching the corner
		for i := 0; i < radius; i++ {
			// Quarter circle: use distance from arc center (radius-i) to
			// compute horizontal fill. i=0 is the tip (narrow), i=radius-1
			// is the base (wide).
			fi := float64(i)
			fr := float64(radius)
			inset := int(fr - math.Sqrt(fi*(2*fr-fi)))
			// Top-left
			drawRect(dst, image.Rect(r.Min.X+inset, r.Min.Y+i, r.Min.X+radius, r.Min.Y+i+1), c, true)
			// Top-right
			drawRect(dst, image.Rect(r.Max.X-radius, r.Min.Y+i, r.Max.X-inset, r.Min.Y+i+1), c, true)
			// Bottom-left
			drawRect(dst, image.Rect(r.Min.X+inset, r.Max.Y-1-i, r.Min.X+radius, r.Max.Y-i), c, true)
			// Bottom-right
			drawRect(dst, image.Rect(r.Max.X-radius, r.Max.Y-1-i, r.Max.X-inset, r.Max.Y-i), c, true)
		}
	} else {
		// Stroked: draw four straight edges and corner arcs
		// Top edge
		drawRect(dst, image.Rect(r.Min.X+radius, r.Min.Y, r.Max.X-radius, r.Min.Y+1), c, true)
		// Bottom edge
		drawRect(dst, image.Rect(r.Min.X+radius, r.Max.Y-1, r.Max.X-radius, r.Max.Y), c, true)
		// Left edge
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y+radius, r.Min.X+1, r.Max.Y-radius), c, true)
		// Right edge
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y+radius, r.Max.X, r.Max.Y-radius), c, true)
		// Corner arcs (1px stroked)
		for i := 0; i < radius; i++ {
			fi := float64(i)
			fr := float64(radius)
			inset := int(fr - math.Sqrt(fi*(2*fr-fi)))
			// Top-left
			drawRect(dst, image.Rect(r.Min.X+inset, r.Min.Y+i, r.Min.X+inset+1, r.Min.Y+i+1), c, true)
			// Top-right
			drawRect(dst, image.Rect(r.Max.X-inset-1, r.Min.Y+i, r.Max.X-inset, r.Min.Y+i+1), c, true)
			// Bottom-left
			drawRect(dst, image.Rect(r.Min.X+inset, r.Max.Y-1-i, r.Min.X+inset+1, r.Max.Y-i), c, true)
			// Bottom-right
			drawRect(dst, image.Rect(r.Max.X-inset-1, r.Max.Y-1-i, r.Max.X-inset, r.Max.Y-i), c, true)
		}
	}
}

// drawPanelShadow draws a subtle shadow behind a panel for depth.
func drawPanelShadow(dst *ebiten.Image, r image.Rectangle, offset int) {
	if r.Empty() || offset <= 0 {
		return
	}
	shadowColor := color.NRGBA{0, 0, 0, 60}
	shadow := image.Rect(r.Min.X+offset, r.Min.Y+offset, r.Max.X+offset, r.Max.Y+offset)
	drawRoundedRect(dst, shadow, shadowColor, 3, true)
}

// drawAccentStripe draws a vertical stripe at the left edge of r.
// Used for instrument color identity on mobile row controls.
// On mobile: 4px wide with 1px vertical inset; desktop: 3px flush.
func drawAccentStripe(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	w := 3
	yTop := r.Min.Y
	yBot := r.Max.Y
	if isSmallScreen() {
		w = 5 // wider stripe on mobile for visibility
		yTop += 2
		yBot -= 2
	}
	stripe := image.Rect(r.Min.X, yTop, r.Min.X+w, yBot)
	drawRect(dst, stripe, col, true)
}

// drawOverflowIcon draws a vertical kebab menu icon (three dots).
var drawOverflowIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	dotSize := max1(minI(r.Dx(), r.Dy()) / 6)
	if dotSize < 2 {
		dotSize = 2
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	spacing := r.Dy() / 4
	for _, dy := range []int{-spacing, 0, spacing} {
		dot := image.Rect(cx-dotSize/2, cy+dy-dotSize/2, cx+dotSize/2, cy+dy+dotSize/2)
		drawRoundedRect(dst, dot, col, dotSize/2, true)
	}
}

// drawRowsIcon draws a horizontal-lines icon representing the drum rows view.
var drawRowsIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	// 3 horizontal lines evenly spaced.
	nLines := 3
	lineH := max1(r.Dy() / 8)
	if lineH < 1 {
		lineH = 1
	}
	totalGap := r.Dy() - nLines*lineH
	gap := totalGap / (nLines + 1)
	if gap < 1 {
		gap = 1
	}
	for i := 0; i < nLines; i++ {
		y := r.Min.Y + gap + i*(lineH+gap)
		drawRect(dst, image.Rect(r.Min.X, y, r.Max.X, y+lineH), col, true)
	}
}

// drawAudioIcon draws an equalizer-bars icon representing the audio/EQ view.
var drawAudioIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	// 5 vertical bars of varying heights (classic EQ visualization).
	nBars := 5
	barW := max1(r.Dx() / (nBars*2 - 1))
	if barW < 1 {
		barW = 1
	}
	gap := barW
	totalW := nBars*barW + (nBars-1)*gap
	startX := r.Min.X + (r.Dx()-totalW)/2
	// Heights as fractions of available height (ascending then descending).
	heights := []int{40, 70, 100, 60, 85}
	for i := 0; i < nBars; i++ {
		x := startX + i*(barW+gap)
		pct := heights[i%len(heights)]
		h := r.Dy() * pct / 100
		if h < 1 {
			h = 1
		}
		y := r.Max.Y - h
		drawRect(dst, image.Rect(x, y, x+barW, r.Max.Y), col, true)
	}
}

// drawChevronUpIcon draws an upward-pointing chevron (^) inside the bounds.
var drawChevronUpIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	thick := maxI(minI(inner.Dx(), inner.Dy())/6, 1)
	cx := (inner.Min.X + inner.Max.X) / 2
	// Chevron tip at vertical center, legs extend down
	tipY := inner.Min.Y + inner.Dy()/3
	legY := inner.Max.Y - inner.Dy()/6
	steps := max1(cx - inner.Min.X)
	// Left leg: bottom-left to center-top
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(inner.Min.X) + t*float64(cx-inner.Min.X)))
		y := int(math.Round(float64(legY) + t*float64(tipY-legY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
	}
	// Right leg: center-top to bottom-right
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(cx) + t*float64(inner.Max.X-cx)))
		y := int(math.Round(float64(tipY) + t*float64(legY-tipY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
	}
}

// drawTrackIcon draws a padlock icon representing follow/auto-scroll state.
// Used for the "follow-on" (locked) state — filled padlock body with shackle.
var drawTrackIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	cx := (inner.Min.X + inner.Max.X) / 2
	// Padlock body (lower 60%)
	bodyTop := inner.Min.Y + inner.Dy()*40/100
	bodyRect := image.Rect(inner.Min.X, bodyTop, inner.Max.X, inner.Max.Y)
	drawRect(dst, bodyRect, col, true)
	// Shackle (closed arc above body)
	thick := maxI(inner.Dx()/6, 1)
	shW := inner.Dx() * 50 / 100
	shTop := inner.Min.Y
	shBot := bodyTop + thick
	// Left arm
	drawRect(dst, image.Rect(cx-shW/2, shTop+thick, cx-shW/2+thick, shBot), col, true)
	// Right arm
	drawRect(dst, image.Rect(cx+shW/2-thick, shTop+thick, cx+shW/2, shBot), col, true)
	// Top bar
	drawRect(dst, image.Rect(cx-shW/2, shTop, cx+shW/2, shTop+thick), col, true)
}

// drawTrackOffIcon draws an unlocked padlock icon for follow-off (free scroll).
var drawTrackOffIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	cx := (inner.Min.X + inner.Max.X) / 2
	// Padlock body (lower 60%, stroked only)
	bodyTop := inner.Min.Y + inner.Dy()*40/100
	bodyRect := image.Rect(inner.Min.X, bodyTop, inner.Max.X, inner.Max.Y)
	drawRect(dst, bodyRect, col, false)
	// Shackle (open — right arm lifted)
	thick := maxI(inner.Dx()/6, 1)
	shW := inner.Dx() * 50 / 100
	shTop := inner.Min.Y
	shBot := bodyTop + thick
	// Left arm
	drawRect(dst, image.Rect(cx-shW/2, shTop+thick, cx-shW/2+thick, shBot), col, true)
	// Right arm (raised — doesn't connect to body)
	drawRect(dst, image.Rect(cx+shW/2-thick, shTop-thick, cx+shW/2, shTop+thick), col, true)
	// Top bar (only left half — open shackle)
	drawRect(dst, image.Rect(cx-shW/2, shTop, cx+shW/2, shTop+thick), col, true)
}

// drawUploadIcon draws an upward-pointing arrow above a horizontal base line.
var drawUploadIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	cx := (inner.Min.X + inner.Max.X) / 2
	thick := maxI(minI(inner.Dx(), inner.Dy())/6, 2)
	// Vertical shaft
	shaftTop := inner.Min.Y + inner.Dy()/4
	shaftBot := inner.Max.Y - inner.Dy()/6
	drawRect(dst, image.Rect(cx-thick/2, shaftTop, cx+thick/2, shaftBot), col, true)
	// Arrowhead (upward chevron)
	armLen := inner.Dx() / 3
	steps := max1(armLen)
	tipY := inner.Min.Y + inner.Dy()/8
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Left arm
		x := int(math.Round(float64(cx) - t*float64(armLen)))
		y := int(math.Round(float64(tipY) + t*float64(shaftTop-tipY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
		// Right arm
		x2 := int(math.Round(float64(cx) + t*float64(armLen)))
		drawRect(dst, image.Rect(x2, y, x2+thick, y+thick), col, true)
	}
	// Base line
	baseY := inner.Max.Y - thick
	drawRect(dst, image.Rect(inner.Min.X, baseY, inner.Max.X, baseY+thick), col, true)
}

// drawImportIcon draws a downward-pointing arrow above a horizontal tray.
var drawImportIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	cx := (inner.Min.X + inner.Max.X) / 2
	thick := maxI(minI(inner.Dx(), inner.Dy())/6, 2)
	// Vertical shaft (downward)
	shaftTop := inner.Min.Y + inner.Dy()/8
	shaftBot := inner.Max.Y - inner.Dy()/3
	drawRect(dst, image.Rect(cx-thick/2, shaftTop, cx+thick/2, shaftBot), col, true)
	// Arrowhead (downward chevron)
	armLen := inner.Dx() / 3
	steps := max1(armLen)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Left arm
		x := int(math.Round(float64(cx) - t*float64(armLen)))
		y := int(math.Round(float64(shaftBot) - t*float64(shaftBot-shaftTop)/2))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
		// Right arm
		x2 := int(math.Round(float64(cx) + t*float64(armLen)))
		drawRect(dst, image.Rect(x2, y, x2+thick, y+thick), col, true)
	}
	// Tray (U-shape): bottom line + short side walls
	trayY := inner.Max.Y - thick
	drawRect(dst, image.Rect(inner.Min.X, trayY, inner.Max.X, trayY+thick), col, true)
	wallH := inner.Dy() / 5
	drawRect(dst, image.Rect(inner.Min.X, trayY-wallH, inner.Min.X+thick, trayY), col, true)
	drawRect(dst, image.Rect(inner.Max.X-thick, trayY-wallH, inner.Max.X, trayY), col, true)
}

// drawExportIcon draws an upward-pointing arrow leaving a horizontal tray.
var drawExportIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	cx := (inner.Min.X + inner.Max.X) / 2
	thick := maxI(minI(inner.Dx(), inner.Dy())/6, 2)
	// Vertical shaft (upward)
	shaftTop := inner.Min.Y + inner.Dy()/8
	shaftBot := inner.Max.Y - inner.Dy()/3
	drawRect(dst, image.Rect(cx-thick/2, shaftTop, cx+thick/2, shaftBot), col, true)
	// Arrowhead (upward chevron)
	armLen := inner.Dx() / 3
	steps := max1(armLen)
	tipY := inner.Min.Y
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(cx) - t*float64(armLen)))
		y := int(math.Round(float64(tipY) + t*float64(shaftTop-tipY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
		x2 := int(math.Round(float64(cx) + t*float64(armLen)))
		drawRect(dst, image.Rect(x2, y, x2+thick, y+thick), col, true)
	}
	// Tray (U-shape): bottom line + short side walls
	trayY := inner.Max.Y - thick
	drawRect(dst, image.Rect(inner.Min.X, trayY, inner.Max.X, trayY+thick), col, true)
	wallH := inner.Dy() / 5
	drawRect(dst, image.Rect(inner.Min.X, trayY-wallH, inner.Min.X+thick, trayY), col, true)
	drawRect(dst, image.Rect(inner.Max.X-thick, trayY-wallH, inner.Max.X, trayY), col, true)
}

// drawChevronDownIcon draws a downward-pointing chevron (v) inside the bounds.
var drawChevronDownIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	inner := insetRect(r, minI(r.Dx(), r.Dy())/5)
	if inner.Empty() {
		return
	}
	thick := maxI(minI(inner.Dx(), inner.Dy())/6, 1)
	cx := (inner.Min.X + inner.Max.X) / 2
	// Chevron tip at bottom, legs extend up
	legY := inner.Min.Y + inner.Dy()/6
	tipY := inner.Max.Y - inner.Dy()/3
	steps := max1(cx - inner.Min.X)
	// Left leg: top-left to center-bottom
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(inner.Min.X) + t*float64(cx-inner.Min.X)))
		y := int(math.Round(float64(legY) + t*float64(tipY-legY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
	}
	// Right leg: center-bottom to top-right
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(cx) + t*float64(inner.Max.X-cx)))
		y := int(math.Round(float64(tipY) + t*float64(legY-tipY)))
		drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
	}
}

// SplitterHandleRect returns the pill handle rect centered at (cx, cy).
// For horizontal dividers (horizontal=true) the pill is wider along X.
// For vertical dividers (horizontal=false) the pill is taller along Y.
func SplitterHandleRect(cx, cy int, horizontal bool) image.Rectangle {
	hLen := SplitterHandleLen()
	hThick := SplitterHandleThick()
	if horizontal {
		x0 := cx - hLen/2
		y0 := cy - hThick/2
		return image.Rect(x0, y0, x0+hLen, y0+hThick)
	}
	x0 := cx - hThick/2
	y0 := cy - hLen/2
	return image.Rect(x0, y0, x0+hThick, y0+hLen)
}

// DrawSplitterHandle draws the pill handle at (cx, cy) using unified colors.
func DrawSplitterHandle(dst *ebiten.Image, cx, cy int, horizontal, hover bool) {
	r := SplitterHandleRect(cx, cy, horizontal)
	col := colSplitterHandle
	if isSmallScreen() {
		col = colSplitterHandleMobile
	}
	if hover {
		col = colSplitterHandleHover
	}
	radius := SplitterHandleThick() / 2
	drawRoundedRect(dst, r, col, radius, true)
	// Draw grip lines on desktop for drag affordance.
	if !isSmallScreen() {
		gripCol := colSplitterGripLine
		if hover {
			gripCol = colSplitterGripLineHover
		}
		drawSplitterGripLines(dst, r, horizontal, gripCol)
	}
}

// drawSplitterGripLines draws 3 thin lines inside the pill, perpendicular
// to the divider direction, as a visual drag affordance.
func drawSplitterGripLines(dst *ebiten.Image, r image.Rectangle, horizontal bool, col color.Color) {
	const nLines = 3
	const lineSpacing = 3
	if horizontal {
		// Horizontal divider → wide pill → draw vertical dashes
		cx := (r.Min.X + r.Max.X) / 2
		totalW := (nLines-1)*lineSpacing + nLines // nLines * 1px + gaps
		startX := cx - totalW/2
		for i := 0; i < nLines; i++ {
			x := startX + i*(1+lineSpacing)
			// Inset top/bottom by 1px for a cleaner look inside the pill
			drawRect(dst, image.Rect(x, r.Min.Y+1, x+1, r.Max.Y-1), col, true)
		}
	} else {
		// Vertical divider → tall pill → draw horizontal dashes
		cy := (r.Min.Y + r.Max.Y) / 2
		totalH := (nLines-1)*lineSpacing + nLines
		startY := cy - totalH/2
		for i := 0; i < nLines; i++ {
			y := startY + i*(1+lineSpacing)
			drawRect(dst, image.Rect(r.Min.X+1, y, r.Max.X-1, y+1), col, true)
		}
	}
}

// popupCornerRadius returns the corner radius for popup panels.
func popupCornerRadius() int {
	if isSmallScreen() {
		return RadiusXL
	}
	return RadiusMD
}

// drawScrim draws a semi-transparent backdrop behind a popup to create depth
// and focus. The scrim covers the full destination image.
func drawScrim(dst *ebiten.Image) {
	b := dst.Bounds()
	drawRect(dst, b, colScrim, true)
}

// drawPanel draws a rounded popup/menu panel with background, border, and
// optional shadow. This is the unified drawing function for all overlay panels.
func drawPanel(dst *ebiten.Image, r image.Rectangle) {
	radius := popupCornerRadius()
	drawPanelShadow(dst, r, 4)
	drawRoundedRect(dst, r, colPanelBG, radius, true)
	drawRoundedRect(dst, r, colPanelBorder, radius, false)
}

// closeButtonRect returns a rect at the top-right corner of panelRect.
// Uses a smaller size on mobile for better proportioning.
func closeButtonRect(panelRect image.Rectangle, pad int) image.Rectangle {
	size := BtnHeightSM
	if !isSmallScreen() {
		size = PopupBtnH()
	}
	return image.Rect(
		panelRect.Max.X-pad-size,
		panelRect.Min.Y+pad,
		panelRect.Max.X-pad,
		panelRect.Min.Y+pad+size,
	)
}

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func absI(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
