package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// One UI goroutine and DrawImage copies the options, so a package scratch is safe; a per-call &op made drawRect
// (thousands of calls a frame) the largest avoidable allocation on WASM.
var drawRectOp ebiten.DrawImageOptions

func drawRectBlit(dst *ebiten.Image, px *ebiten.Image, sx, sy, tx, ty float64) {
	bumpDrawCall()
	drawRectOp.GeoM.Reset()
	drawRectOp.GeoM.Scale(sx, sy)
	drawRectOp.GeoM.Translate(tx, ty)
	dst.DrawImage(px, &drawRectOp)
}

// A var so tests can capture draw calls.
var drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
	if r.Empty() {
		return
	}
	// Scaled 1x1 pixel blits avoid vector-path overhead, much faster on WASM.
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

// Fixed-colour strips are served from pixelCache, so after the first frame it allocates nothing.
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

// Flat rects: a rounded rect emits 35+ DrawImage calls vs 6, and Ebiten's dependency tracking scales O(N²)
// with draw calls. topEdgeHighlight is a parameter so drawButton reads no globals.
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

// The composite is cached as a sprite, so each button is one blit.
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
func popupButtonRadius() int { return RadiusControl }

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

// Tests override the per-icon function vars to count calls.

// cornerSpriteKey identifies a cached corner arc sprite.
type cornerSpriteKey struct {
	radius int
	rgba   uint32
	filled bool
}

// Only the top-left quadrant is stored; the other corners are drawn flipped.
var cornerSpriteCache = map[cornerSpriteKey]*ebiten.Image{}

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

// A quarter of the shorter side; shared by the drawRoundedRect clamp and TestNoPillShapedControls.
func maxRoundedRadius(w, h int) int {
	return minI(w, h) / 4
}

// For shared helpers serving rects of every size: the role picks the intent, the geometry caps it. A call
// site drawing ONE known control names its role token directly.
func fitRadius(role int, r image.Rectangle) int {
	if ceil := maxRoundedRadius(r.Dx(), r.Dy()); role > ceil {
		return ceil
	}
	return role
}

// The ONE escape from the anti-pill ceiling, a sentinel so no plausible number can reach it by accident.
const radiusCapsule = -1

// Drag handles and slider rails are capsules by design. Routed through drawRoundedRect so there stays one
// interceptable rounded-drawing chokepoint.
func drawCapsule(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
	drawRoundedRect(dst, r, c, radiusCapsule, filled)
}

// Sprite-cached corners: 7 DrawImage calls instead of 3 + 4×radius. The radius is clamped to the anti-pill
// ceiling; a shape that needs semicircular ends is a capsule.
var drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
	if r.Empty() {
		return
	}
	// The runtime backstop: no pill reaches the screen whatever a call site asks; TestNoPillShapedControls
	// intercepts this var to catch the over-asking site.
	if radius == radiusCapsule {
		radius = minI(r.Dx(), r.Dy()) / 2
	} else if maxR := maxRoundedRadius(r.Dx(), r.Dy()); radius > maxR {
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
	drawRoundedRect(dst, shadow, shadowColor, fitRadius(popupCornerRadius(), shadow), true)
}

// (icon implementations live in drawing_icons.go)

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

// One render path on every platform; only the colour differs (gray desktop, cyan mobile).
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

func drawScrim(dst *ebiten.Image) {
	b := dst.Bounds()
	drawRect(dst, b, colScrim, true)
}

// The single draw path for every overlay panel.
func drawPanel(dst *ebiten.Image, r image.Rectangle) {
	radius := fitRadius(popupCornerRadius(), r)
	drawPanelShadow(dst, r, 6)
	drawRoundedRect(dst, r, colPanelBG, radius, true)
	drawRoundedRect(dst, r, colPanelBorder, radius, false)
}

// Top-only rounding: the rect extends past the bottom, so the bottom corners fall off-screen.
func drawBottomSheetPanel(dst *ebiten.Image, r image.Rectangle) {
	// The same helper paints the ~38 px mobile action bar, where sheet corners would eat half its height, so the
	// ceiling is computed against the visible r.
	radius := fitRadius(RadiusSheet, r)
	// Shadow offset at top only (bottom is flush with screen edge).
	shadowColor := color.NRGBA{0, 0, 0, 80}
	shadow := image.Rect(r.Min.X+6, r.Min.Y+6, r.Max.X+6, r.Max.Y)
	drawRoundedRect(dst, shadow, shadowColor, fitRadius(radius, shadow), true)

	extended := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y+radius)
	drawRoundedRect(dst, extended, colPanelBG, radius, true)

	// No bottom border: the sheet is flush with the screen bottom.
	drawRoundedRect(dst, extended, colPanelBorder, radius, false)
}

// The midline is 0 dB, the edges ±24 dB. Rasterised with drawRect, not vector.Path, so it renders under the
// test build's stubbed DrawTriangles.
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

// Muted, never red: red is reserved for stop/error text and destructive fills, and a dismiss is neither.
func closeIconColor() color.Color { return colTextSecondary }

// Smaller on mobile.
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
