//go:build test

package ebiten

import (
	"errors"
	"image"
	"image/color"
)

// Termination is returned from Update() to signal a clean exit from RunGame.
var Termination = errors.New("regular termination")

type Image struct {
	w, h int
	pix  []color.RGBA
	// clip, when non-empty, makes this Image a SubImage VIEW: it shares the
	// parent's pix (same w/h stride, same coordinate system — exactly real
	// ebiten's SubImage semantics) and every read/write is clipped to clip.
	clip image.Rectangle
}

// bounds is the image's addressable rect in its own (= parent's) coordinates.
func (i *Image) bounds() image.Rectangle {
	if !i.clip.Empty() {
		return i.clip
	}
	return image.Rect(0, 0, i.w, i.h)
}

// in reports whether (x,y) is addressable (inside the view clip and the
// backing store).
func (i *Image) in(x, y int) bool {
	return image.Pt(x, y).In(i.bounds()) && x >= 0 && y >= 0 && x < i.w && y < i.h
}

func NewImage(w, h int) *Image {
	return &Image{w: w, h: h, pix: make([]color.RGBA, w*h)}
}
func NewImageFromImage(img image.Image) *Image {
	b := img.Bounds()
	out := NewImage(b.Dx(), b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.pix[y*out.w+x] = color.RGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA)
		}
	}
	return out
}

// DrawImageObserver, when set (tests only), is called for every DrawImage
// with the destination, source and options — the seam theme/provenance tests
// use to attribute a blitted sprite (and its tint) to its call site.
var DrawImageObserver func(dst, src *Image, opts *DrawImageOptions)

func (i *Image) DrawImage(src *Image, opts *DrawImageOptions) {
	if i == nil || src == nil || len(i.pix) == 0 || len(src.pix) == 0 {
		return
	}
	if opts == nil {
		opts = &DrawImageOptions{}
	}
	if DrawImageObserver != nil {
		DrawImageObserver(i, src, opts)
	}
	// Basic scale + translate handling; rotation is ignored for test stubs.
	scaleX, scaleY := opts.GeoM.scaleX, opts.GeoM.scaleY
	if scaleX == 0 {
		scaleX = 1
	}
	if scaleY == 0 {
		scaleY = 1
	}
	// The source region is src's bounds (a SubImage view draws only its
	// rect, with that rect's Min as the source origin — real ebiten).
	sb := src.bounds()
	w := int(float64(sb.Dx()) * scaleX)
	h := int(float64(sb.Dy()) * scaleY)
	if w <= 0 || h <= 0 {
		return
	}
	tx := int(opts.GeoM.translateX)
	ty := int(opts.GeoM.translateY)
	// Clip once: the destination's addressable rect (a view's clip ∩ the
	// backing store) intersected with where the source lands.
	clip := i.bounds().Intersect(image.Rect(0, 0, i.w, i.h))
	// Unscaled multi-pixel blit (cached-surface draws): copy per-pixel so
	// pixel-identity tests see real content. Scaled draws keep the legacy
	// first-pixel fill (the 1x1 stretched drawRectBlit pattern).
	if scaleX == 1 && scaleY == 1 && sb.Dx()*sb.Dy() > 1 {
		dr := image.Rect(tx, ty, tx+sb.Dx(), ty+sb.Dy()).Intersect(clip)
		tint := opts.ColorScale.set && FaithfulCompositing
		for dy := dr.Min.Y; dy < dr.Max.Y; dy++ {
			srow := (sb.Min.Y+dy-ty)*src.w + sb.Min.X - tx
			drow := dy * i.w
			for dx := dr.Min.X; dx < dr.Max.X; dx++ {
				c := src.pix[srow+dx]
				if c.A == 0 {
					continue // preserve dst where the source is transparent
				}
				if tint {
					c = opts.ColorScale.apply(c)
				}
				i.pix[drow+dx] = c
			}
		}
		return
	}
	// Copy the first pixel of src (common case for 1x1 cached pixels).
	c := opts.ColorScale.apply(src.pix[sb.Min.Y*src.w+sb.Min.X])
	dr := image.Rect(tx, ty, tx+w, ty+h).Intersect(clip)
	for dy := dr.Min.Y; dy < dr.Max.Y; dy++ {
		row := i.pix[dy*i.w+dr.Min.X : dy*i.w+dr.Max.X]
		for k := range row {
			row[k] = c
		}
	}
}
func (i *Image) Fill(c color.Color) {
	if i == nil {
		return
	}
	i.fillWith(color.RGBAModel.Convert(c).(color.RGBA))
}

// fillWith writes c over the image's addressable rect (a view fills only its
// clip, never the whole shared parent).
func (i *Image) fillWith(c color.RGBA) {
	if len(i.pix) == 0 {
		return
	}
	if i.clip.Empty() {
		for idx := range i.pix {
			i.pix[idx] = c
		}
		return
	}
	b := i.bounds().Intersect(image.Rect(0, 0, i.w, i.h))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i.pix[y*i.w+x] = c
		}
	}
}

func (i *Image) Clear() {
	if i == nil {
		return
	}
	i.fillWith(color.RGBA{})
}

// Deallocate releases the image's internal storage. The Image object
// remains valid; ebiten lazily reallocates on next use. Mirrors the
// ebiten v2.8.8 API so production code that calls Deallocate to
// reclaim atlas slots compiles under the test stub as well.
func (i *Image) Deallocate() {
	if i == nil {
		return
	}
	i.pix = nil
}

func (i *Image) Bounds() image.Rectangle { return i.bounds() }

// FaithfulCompositing (tests only) turns on real-ebiten compositing semantics
// in the stub: SubImage returns a pixel-SHARING view and DrawImage applies the
// ColorScale tint. Off by default — the stub then discards draws into a
// sub-image and ignores tints, which is what the thousands of non-pixel tests
// (soak, alloc, perf budgets) were calibrated against and is far cheaper per
// frame. Pixel guards that must SEE every zone (the theme-drift guards) turn
// it on for their duration.
var FaithfulCompositing bool

// SubImage mirrors real ebiten: a small view header (NO pixel allocation —
// byte-budget tests stay honest) that, under FaithfulCompositing, SHARES the
// parent's pixels, keeps the parent's coordinate system, and clips every read
// and write to r. Without it the view is detached and draws into it are
// discarded — so pixel tests that leave the flag off cannot see anything drawn
// through a clipped zone (the reason the original theme-parity test never
// caught a stale cached transport toolbar).
func (i *Image) SubImage(r image.Rectangle) image.Image {
	if i == nil {
		return &Image{}
	}
	if !FaithfulCompositing {
		// Legacy stub semantics: a detached, pixel-less view (draws no-op).
		return &Image{w: r.Dx(), h: r.Dy()}
	}
	clip := r.Intersect(i.bounds())
	if clip.Empty() {
		// An empty view: reads transparent, writes no-op.
		return &Image{w: i.w, h: i.h, clip: image.Rect(-1, -1, -1, -1)}
	}
	return &Image{w: i.w, h: i.h, pix: i.pix, clip: clip}
}
func (i *Image) Size() (int, int)        { b := i.bounds(); return b.Dx(), b.Dy() }
func (i *Image) ColorModel() color.Model { return color.RGBAModel }
func (i *Image) At(x, y int) color.Color {
	// pix == nil after Deallocate: reads stay safe (ebiten lazily
	// reallocates on next use; the stub reports transparent).
	if i == nil || i.pix == nil || !i.in(x, y) {
		return color.RGBA{}
	}
	return i.pix[y*i.w+x]
}

var (
	MockCursorX, MockCursorY int
	MousePressed             = map[MouseButton]bool{}
	KeysPressed              = map[Key]bool{}
	Chars                    []rune
)

func CursorPosition() (int, int)              { return MockCursorX, MockCursorY }
func IsMouseButtonPressed(b MouseButton) bool { return MousePressed[b] }
func IsKeyPressed(k Key) bool                 { return KeysPressed[k] }
func InputChars() []rune                      { c := Chars; Chars = nil; return c }
func Wheel() (float64, float64)               { return 0, 0 }
func ScreenSizeInFullscreen() (int, int)      { return 0, 0 }

// Touch support
type TouchID int

var (
	MockTouches = map[TouchID]struct{ X, Y int }{}
)

func TouchIDs() []TouchID {
	ids := make([]TouchID, 0, len(MockTouches))
	for id := range MockTouches {
		ids = append(ids, id)
	}
	return ids
}

func TouchPosition(id TouchID) (int, int) {
	if pos, ok := MockTouches[id]; ok {
		return pos.X, pos.Y
	}
	return 0, 0
}

// ColorScale mirrors Ebiten's ColorScale: a multiplicative tint whose zero
// value is the identity. Real ebiten applies it to the premultiplied source
// color as (r·sr·sa, g·sg·sa, b·sb·sa, a·sa) — the convention every call site
// here follows (it passes unpremultiplied ratios plus alpha). The stub used to
// ignore it, so tinted glyphs (text, icons) rendered in the mask's own color
// and a stale tint was invisible to pixel tests.
type ColorScale struct {
	r, g, b, a float32
	set        bool
}

// Scale multiplies the tint.
func (c *ColorScale) Scale(r, g, b, a float32) {
	if !c.set {
		c.r, c.g, c.b, c.a, c.set = 1, 1, 1, 1, true
	}
	c.r *= r
	c.g *= g
	c.b *= b
	c.a *= a
}

// Factors returns the tint multipliers and whether any tint is set. Tests only.
// ScaleWithColor multiplies the scale by clr's premultiplied components.
func (c *ColorScale) ScaleWithColor(clr color.Color) {
	cr, cg, cb, ca := clr.RGBA()
	c.Scale(float32(cr)/0xffff, float32(cg)/0xffff, float32(cb)/0xffff, float32(ca)/0xffff)
}

func (c ColorScale) Factors() (r, g, b, a float32, ok bool) {
	if !c.set {
		return 1, 1, 1, 1, false
	}
	return c.r, c.g, c.b, c.a, true
}

// apply tints a premultiplied source pixel (FaithfulCompositing only).
func (c ColorScale) apply(p color.RGBA) color.RGBA {
	if !c.set || !FaithfulCompositing {
		return p
	}
	cl := func(v float32) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	return color.RGBA{
		R: cl(float32(p.R) * c.r * c.a),
		G: cl(float32(p.G) * c.g * c.a),
		B: cl(float32(p.B) * c.b * c.a),
		A: cl(float32(p.A) * c.a),
	}
}

// Drawing options
type DrawImageOptions struct {
	GeoM       GeoM
	ColorScale ColorScale
}

// Constants
type MouseButton int

const (
	MouseButtonLeft MouseButton = iota
	MouseButtonRight
)

type Key int

const (
	KeyShiftLeft Key = iota
	KeyShiftRight
	KeyBackspace
	KeyS
	KeyEnter
	KeyEscape
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeySlash
	KeyControlLeft
	KeyControlRight
	KeyMetaLeft
	KeyMetaRight
	KeyZ
	KeyY
	KeySpace
	KeyArrowLeft
	KeyArrowRight
	KeyArrowUp
	KeyArrowDown
	KeyBracketLeft
	KeyBracketRight
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	KeyEqual
	KeyMinus
	KeyNumpadAdd
	KeyNumpadSubtract
	KeyC
	KeyX
	KeyV
	KeyR
	KeyPeriod
	KeyComma
	KeyDelete
	KeyTab
)

// Window and run stubs
type Game interface {
	Update() error
	Draw(*Image)
	Layout(int, int) (int, int)
}

const SyncWithFPS = -1

func SetWindowSize(w, h int)       {}
func SetWindowTitle(title string)  {}
func SetTPS(tps int)               {}
func SetVsyncEnabled(enabled bool) {}
func RunGame(g Game) error         { return nil }

// CursorShapeType represents a shape of a mouse cursor.
type CursorShapeType int

const (
	CursorShapeDefault CursorShapeType = iota
	CursorShapeText
	CursorShapeCrosshair
	CursorShapePointer
	CursorShapeEWResize
	CursorShapeNSResize
	CursorShapeNESWResize
	CursorShapeNWSEResize
	CursorShapeMove
	CursorShapeNotAllowed
)

var mockCursorShape CursorShapeType

func SetCursorShape(shape CursorShapeType) { mockCursorShape = shape }
func CursorShape() CursorShapeType         { return mockCursorShape }

// ─── Triangle / vector primitives (test stubs) ─────────────────────────
//
// The icon renderer (icon_renderer.go) uses Vertex / DrawTriangles to
// rasterize antialiased paths. Under the test build none of this actually
// renders — the stubs exist so the test build compiles. Field shapes
// match real Ebiten v2.8.8 so call sites compile against either.

// Vertex matches ebiten.Vertex's public fields.
type Vertex struct {
	DstX, DstY float32
	SrcX, SrcY float32

	ColorR, ColorG, ColorB, ColorA float32

	Custom0, Custom1, Custom2, Custom3 float32
}

// FillRule mirrors the real enum.
type FillRule int

const (
	FillRuleFillAll FillRule = iota
	FillRuleNonZero
	FillRuleEvenOdd
)

// ColorScaleMode mirrors the real enum.
type ColorScaleMode int

const (
	ColorScaleModeStraightAlpha ColorScaleMode = iota
	ColorScaleModePremultipliedAlpha
)

// DrawTrianglesOptions matches the real shape (only the fields the icon
// renderer actually sets are exercised; others kept for source-compat).
type DrawTrianglesOptions struct {
	ColorScaleMode ColorScaleMode
	Address        Address
	FillRule       FillRule
	AntiAlias      bool
}

// Address mirrors the real enum.
type Address int

const (
	AddressUnsafe Address = iota
	AddressClampToZero
	AddressRepeat
)

// DrawTriangles is a no-op stub — the test build cannot rasterize paths.
func (i *Image) DrawTriangles(vertices []Vertex, indices []uint16, src *Image, options *DrawTrianglesOptions) {
}

// SharesPixels reports whether a and b are views over the same pixel store
// (an image and its SubImage views). Tests only.
func SharesPixels(a, b *Image) bool {
	return a != nil && b != nil && len(a.pix) > 0 && len(b.pix) > 0 && &a.pix[0] == &b.pix[0]
}

// Translation returns the GeoM translation (tests only).
func (o *DrawImageOptions) Translation() (int, int) {
	return int(o.GeoM.translateX), int(o.GeoM.translateY)
}
