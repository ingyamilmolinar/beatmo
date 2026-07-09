package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawRectOp is the reusable DrawImageOptions for drawRect's pixel blits.
// Update + Draw run on a single UI goroutine (same reuse contract as the
// drawArcVS/drawArcIS scratch buffers in knob.go), and ebiten's DrawImage
// copies the options before returning, so a package-level scratch is safe.
// Pre-fix every drawRect call heap-allocated its options struct via the
// escaping &op — drawRect is the universal fill primitive (thousands of
// calls per frame on the audio-panel tabs), making it the single largest
// avoidable per-frame allocation source on WASM where GC starvation is the
// binding constraint.
var drawRectOp ebiten.DrawImageOptions

// drawRectBlit blits the 1x1 pixel px scaled to (sx, sy) at (tx, ty) using
// the shared drawRectOp scratch.
func drawRectBlit(dst *ebiten.Image, px *ebiten.Image, sx, sy, tx, ty float64) {
	bumpDrawCall()
	drawRectOp.GeoM.Reset()
	drawRectOp.GeoM.Scale(sx, sy)
	drawRectOp.GeoM.Translate(tx, ty)
	dst.DrawImage(px, &drawRectOp)
}

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
		drawRectBlit(dst, px, float64(r.Dx()), float64(r.Dy()), float64(r.Min.X), float64(r.Min.Y))
		return
	}
	// 1px stroke on each side: top, bottom, left, right.
	drawRectBlit(dst, px, float64(r.Dx()), 1, float64(r.Min.X), float64(r.Min.Y))
	drawRectBlit(dst, px, float64(r.Dx()), 1, float64(r.Min.X), float64(r.Max.Y-1))
	drawRectBlit(dst, px, 1, float64(r.Dy()), float64(r.Min.X), float64(r.Min.Y))
	drawRectBlit(dst, px, 1, float64(r.Dy()), float64(r.Max.X-1), float64(r.Min.Y))
}

// fillVerticalGradient paints rect with a smooth top→bottom gradient from
// topCol to botCol as a stack of `bands` horizontal strips. Each strip is a
// solid drawRect of a fixed interpolated color, so after the first frame the
// per-band colors are served from pixelCache and the fill adds no per-frame
// allocations. ~32 bands is visually indistinguishable from a true gradient
// at panel scale while staying cheap. Used for the sunset grid-pane backdrop
// (DESIGN.md colors.grid-horizon).
func fillVerticalGradient(dst *ebiten.Image, rect image.Rectangle, topCol, botCol color.Color, bands int) {
	if rect.Empty() || bands < 1 {
		return
	}
	tr, tg, tb, ta := topCol.RGBA()
	br, bg, bb, ba := botCol.RGBA()
	lerp := func(a, b uint32, t float64) uint8 {
		// a,b are 16-bit (0..65535); /257 maps back to 8-bit (0..255).
		return uint8((float64(a)*(1-t) + float64(b)*t) / 257.0)
	}
	h := rect.Dy()
	for i := 0; i < bands; i++ {
		y0 := rect.Min.Y + h*i/bands
		y1 := rect.Min.Y + h*(i+1)/bands
		if y1 <= y0 {
			continue
		}
		t := 0.0
		if bands > 1 {
			t = float64(i) / float64(bands-1)
		}
		c := color.RGBA{
			R: lerp(tr, br, t),
			G: lerp(tg, bg, t),
			B: lerp(tb, bb, t),
			A: lerp(ta, ba, t),
		}
		drawRect(dst, image.Rect(rect.Min.X, y0, rect.Max.X, y1), c, true)
	}
}

// drawButton renders a filled rectangle with a border. It can be overridden in tests.
// Uses flat rects for performance — rounded corners are reserved for cached
// overlays/popups via drawRoundedButton. Each drawRoundedRect(radius=8) emits
// 35+ DrawImage calls vs 6 for flat rects, and Ebiten's dependency-tracking
// map iteration (runtime.mapiternext) scales O(N²) with draw-call count.
//
// topEdgeHighlight is a pure parameter: callers decide (via Profile() or
// spec.TopEdgeHighlight at the boundary) whether to draw the 1-px depth
// accent. Keeping the flag at the parameter line means drawButton itself
// reads no globals — Render(spec, state) can be a pure function w.r.t. its
// inputs once it has hoisted the Profile() read to its entry point.
var drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed, topEdgeHighlight bool) {
	if r.Empty() {
		return
	}
	fc := fill
	if pressed {
		fc = adjustColor(fill, -20)
	}
	drawRect(dst, r, fc, true)
	// Subtle top-edge highlight for depth (skip on mobile for flat look).
	if topEdgeHighlight {
		highlight := adjustColor(fc, 10)
		drawRect(dst, image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+2), highlight, true)
	}
	drawRect(dst, r, border, false)
}

// roundedBtnKey identifies a cached rounded button sprite.
type roundedBtnKey struct {
	w, h   int
	fill   uint32
	border uint32
	radius int
}

var roundedBtnCache = map[roundedBtnKey]*ebiten.Image{}

// drawRoundedButton renders a filled rounded rectangle with a border.
// Same as drawButton but uses drawRoundedRect for fill and border.
// Caches the composite output as a sprite so each button is 1 DrawImage blit.
var drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
	if r.Empty() {
		return
	}
	fc := fill
	if pressed {
		fc = adjustColor(fill, -20)
	}
	k := roundedBtnKey{
		w: r.Dx(), h: r.Dy(),
		fill: packRGBA(fc), border: packRGBA(border),
		radius: radius,
	}
	if spr, ok := roundedBtnCache[k]; ok {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(spr, &op)
		return
	}
	spr := ebiten.NewImage(r.Dx(), r.Dy())
	zr := image.Rect(0, 0, r.Dx(), r.Dy())
	drawRoundedRect(spr, zr, fc, radius, true)
	drawRoundedRect(spr, zr, border, radius, false)
	roundedBtnCache[k] = spr
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(spr, &op)
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

// blendColor linearly interpolates between base and accent by factor t (0..1).
func blendColor(base, accent color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(base.R) + float64(int(accent.R)-int(base.R))*t),
		G: uint8(float64(base.G) + float64(int(accent.G)-int(base.G))*t),
		B: uint8(float64(base.B) + float64(int(accent.B)-int(base.B))*t),
		A: 255,
	}
}

// Icon implementations live in drawing_icons.go. The IconID dispatch in
// icons.go reaches the per-icon function-vars defined there; tests
// override those vars to capture call counts.

// cornerSpriteKey identifies a cached corner arc sprite.
type cornerSpriteKey struct {
	radius int
	rgba   uint32
	filled bool
}

// cornerSpriteCache caches pre-rendered corner arc sprites keyed by
// (radius, color, filled). Each sprite is a radius×radius image containing
// the top-left quadrant; the other 3 corners are drawn by flipping.
var cornerSpriteCache = map[cornerSpriteKey]*ebiten.Image{}

// cornerSprite returns a cached radius×radius image containing a filled or
// stroked top-left corner arc. The image is pre-rendered once and reused.
func cornerSprite(radius int, c color.Color, filled bool) *ebiten.Image {
	k := cornerSpriteKey{radius: radius, rgba: packRGBA(c), filled: filled}
	if img, ok := cornerSpriteCache[k]; ok {
		return img
	}
	img := ebiten.NewImage(radius, radius)
	fr := float64(radius)
	if filled {
		for i := 0; i < radius; i++ {
			fi := float64(i)
			inset := int(fr - math.Sqrt(fi*(2*fr-fi)))
			drawRect(img, image.Rect(inset, i, radius, i+1), c, true)
		}
	} else {
		for i := 0; i < radius; i++ {
			fi := float64(i)
			inset := int(fr - math.Sqrt(fi*(2*fr-fi)))
			drawRect(img, image.Rect(inset, i, inset+1, i+1), c, true)
		}
	}
	cornerSpriteCache[k] = img
	return img
}

// drawRoundedRect draws a filled or stroked rectangle with rounded corners.
// Uses sprite-cached corner arcs: the 4 corners are pre-rendered as small
// radius×radius images and blitted with flips, reducing DrawImage calls from
// 3 + 4×radius (filled) to 7 (body + 4 corners).
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

	spr := cornerSprite(radius, c, filled)

	if filled {
		// Body: 3 rects (center + top strip + bottom strip)
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y+radius, r.Max.X, r.Max.Y-radius), c, true)
		drawRect(dst, image.Rect(r.Min.X+radius, r.Min.Y, r.Max.X-radius, r.Min.Y+radius), c, true)
		drawRect(dst, image.Rect(r.Min.X+radius, r.Max.Y-radius, r.Max.X-radius, r.Max.Y), c, true)
	} else {
		// Edges: 4 rects
		drawRect(dst, image.Rect(r.Min.X+radius, r.Min.Y, r.Max.X-radius, r.Min.Y+1), c, true)
		drawRect(dst, image.Rect(r.Min.X+radius, r.Max.Y-1, r.Max.X-radius, r.Max.Y), c, true)
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y+radius, r.Min.X+1, r.Max.Y-radius), c, true)
		drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y+radius, r.Max.X, r.Max.Y-radius), c, true)
	}

	// Top-left corner (as-is)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(spr, &op)

	// Top-right corner (flip X)
	op.GeoM.Reset()
	op.GeoM.Scale(-1, 1)
	op.GeoM.Translate(float64(r.Max.X), float64(r.Min.Y))
	dst.DrawImage(spr, &op)

	// Bottom-left corner (flip Y)
	op.GeoM.Reset()
	op.GeoM.Scale(1, -1)
	op.GeoM.Translate(float64(r.Min.X), float64(r.Max.Y))
	dst.DrawImage(spr, &op)

	// Bottom-right corner (flip X+Y)
	op.GeoM.Reset()
	op.GeoM.Scale(-1, -1)
	op.GeoM.Translate(float64(r.Max.X), float64(r.Max.Y))
	dst.DrawImage(spr, &op)
}

// drawPanelShadow draws a subtle shadow behind a panel for depth.
func drawPanelShadow(dst *ebiten.Image, r image.Rectangle, offset int) {
	if r.Empty() || offset <= 0 {
		return
	}
	shadowColor := color.NRGBA{0, 0, 0, 80}
	shadow := image.Rect(r.Min.X+offset, r.Min.Y+offset, r.Max.X+offset, r.Max.Y+offset)
	drawRoundedRect(dst, shadow, shadowColor, popupCornerRadius(), true)
}

// drawAccentStripe draws a vertical stripe at the left edge of r.
// Used for instrument color identity on mobile row controls.
// On mobile: 4px wide with 1px vertical inset; desktop: 3px flush.
// Skips silently when col is nil (rare test paths build DrumRow values
// directly without going through AddRow's color allocator).
func drawAccentStripe(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() || col == nil {
		return
	}
	p := Profile()
	w := p.AccentStripeWidth
	yTop := r.Min.Y + p.AccentStripeInsetY
	yBot := r.Max.Y - p.AccentStripeInsetY
	stripe := image.Rect(r.Min.X, yTop, r.Min.X+w, yBot)
	drawRect(dst, stripe, col, true)
}

// (icon implementations live in drawing_icons.go)

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

// DrawSplitterHandle draws the divider handle at (cx, cy) using a single
// unified render path on every platform: a square (sharp-cornered) handle
// with a soft glow halo behind it. The former desktop-only grip-line variant
// was retired so the same component renders identically everywhere; only the
// handle color stays platform-parameterized (gray desktop / cyan mobile).
func DrawSplitterHandle(dst *ebiten.Image, cx, cy int, horizontal, hover bool) {
	r := SplitterHandleRect(cx, cy, horizontal)
	col := Profile().SplitterHandleColor
	if hover {
		col = colSplitterHandleHover
	}

	// Soft glow/halo: a wider, lower-opacity square behind the handle.
	const glowExtra = 8
	glowR := image.Rect(r.Min.X-glowExtra/2, r.Min.Y-glowExtra/2, r.Max.X+glowExtra/2, r.Max.Y+glowExtra/2)
	cr, cg, cb, ca := col.RGBA()
	glowAlpha := uint8(float64(ca>>8) * 0.3)
	glowCol := color.NRGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), glowAlpha}
	drawRect(dst, glowR, glowCol, true)

	// Square handle (sharp corners) — unified across platforms.
	drawRect(dst, r, col, true)
}

// popupCornerRadius returns the corner radius for popup panels.
func popupCornerRadius() int { return Profile().PopupCornerRadius }

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
	drawPanelShadow(dst, r, 6)
	drawRoundedRect(dst, r, colPanelBG, radius, true)
	drawRoundedRect(dst, r, colPanelBorder, radius, false)
}

// drawBottomSheetPanel draws a panel for mobile bottom sheets with rounded top
// corners (RadiusXL) and flat bottom corners (flush with screen bottom).
// Achieves top-only rounding by drawing a rounded rect that extends past the
// bottom of the destination, so the bottom corners are naturally clipped.
func drawBottomSheetPanel(dst *ebiten.Image, r image.Rectangle) {
	radius := RadiusXL
	// Shadow offset at top only (bottom is flush with screen edge).
	shadowColor := color.NRGBA{0, 0, 0, 80}
	shadow := image.Rect(r.Min.X+6, r.Min.Y+6, r.Max.X+6, r.Max.Y)
	drawRoundedRect(dst, shadow, shadowColor, radius, true)

	// Extend the rect below by radius so the bottom corners are drawn as
	// straight edges (the rounded bottom corners fall off-screen).
	extended := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y+radius)
	drawRoundedRect(dst, extended, colPanelBG, radius, true)

	// Border: draw top rounded edge + sides only (skip bottom border
	// since the sheet is flush with screen bottom).
	drawRoundedRect(dst, extended, colPanelBorder, radius, false)
}

// drawSparklineInRect renders dB samples (expected range ±24) as a connected
// polyline within r, using col. Used by the mobile EQ peek strip
// (drumview_draw.go) to expose the EQ shape at a glance — the polyline
// midline corresponds to 0 dB and the rect's top/bottom edges to ±24 dB.
//
// Implementation rasterizes the polyline as a chain of 1×1 filled rects
// via drawRect rather than vector.Path so it renders under the test
// build's stubbed DrawTriangles (the icon vector pipeline is a no-op
// there).
func drawSparklineInRect(dst *ebiten.Image, r image.Rectangle, samples []float64, col color.Color) {
	if len(samples) < 2 || r.Empty() {
		return
	}
	const dBRange = 24.0
	mid := r.Min.Y + r.Dy()/2
	half := float64(r.Dy() / 2)
	clampY := func(y int) int {
		if y < r.Min.Y {
			return r.Min.Y
		}
		if y >= r.Max.Y {
			return r.Max.Y - 1
		}
		return y
	}
	prevX := r.Min.X
	prevY := clampY(mid - int(samples[0]/dBRange*half))
	for i := 1; i < len(samples); i++ {
		x := r.Min.X + (r.Dx()*i)/(len(samples)-1)
		y := clampY(mid - int(samples[i]/dBRange*half))
		drawSparkLineSeg(dst, prevX, prevY, x, y, col)
		prevX, prevY = x, y
	}
}

// drawSparkLineSeg rasterizes a line from (x0,y0) to (x1,y1) as a chain
// of 1×1 pixel rects using Bresenham's algorithm.
func drawSparkLineSeg(dst *ebiten.Image, x0, y0, x1, y1 int, col color.Color) {
	dx := absI(x1 - x0)
	dy := absI(y1 - y0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx - dy
	for {
		drawRect(dst, image.Rect(x0, y0, x0+1, y0+1), col, true)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// closeIconColor is the shared tint for every pop-up close "X" glyph:
// on-surface-muted, per the DESIGN.md IconColor mapping ("Close (any panel)
// → on-surface-muted"). Red stays reserved for stop/error text and
// destructive fills (three-reds invariant) so a neutral dismiss never reads
// as destructive.
func closeIconColor() color.Color { return colTextSecondary }

// closeButtonRect returns a rect at the top-right corner of panelRect.
// Uses a smaller size on mobile for better proportioning.
func closeButtonRect(panelRect image.Rectangle, pad int) image.Rectangle {
	size := Profile().CloseButtonSize
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
