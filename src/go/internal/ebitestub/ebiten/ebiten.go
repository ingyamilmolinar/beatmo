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

func (i *Image) DrawImage(src *Image, opts *DrawImageOptions) {
	if i == nil || src == nil {
		return
	}
	if opts == nil {
		opts = &DrawImageOptions{}
	}
	// Basic scale + translate handling; rotation is ignored for test stubs.
	scaleX, scaleY := opts.GeoM.scaleX, opts.GeoM.scaleY
	if scaleX == 0 {
		scaleX = 1
	}
	if scaleY == 0 {
		scaleY = 1
	}
	w := int(float64(src.w) * scaleX)
	h := int(float64(src.h) * scaleY)
	if w <= 0 || h <= 0 {
		return
	}
	tx := int(opts.GeoM.translateX)
	ty := int(opts.GeoM.translateY)
	// Copy the first pixel of src (common case for 1x1 cached pixels).
	var c color.RGBA
	if len(src.pix) > 0 {
		c = src.pix[0]
	}
	for y := 0; y < h; y++ {
		dstY := ty + y
		if dstY < 0 || dstY >= i.h {
			continue
		}
		for x := 0; x < w; x++ {
			dstX := tx + x
			if dstX < 0 || dstX >= i.w {
				continue
			}
			i.pix[dstY*i.w+dstX] = c
		}
	}
}
func (i *Image) Fill(c color.Color) {
	if i == nil {
		return
	}
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	for idx := range i.pix {
		i.pix[idx] = rgba
	}
}

func (i *Image) Clear() {
	if i == nil {
		return
	}
	for idx := range i.pix {
		i.pix[idx] = color.RGBA{}
	}
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

func (i *Image) Bounds() image.Rectangle { return image.Rect(0, 0, i.w, i.h) }
func (i *Image) SubImage(r image.Rectangle) image.Image {
	return &Image{w: r.Dx(), h: r.Dy(), pix: make([]color.RGBA, r.Dx()*r.Dy())}
}
func (i *Image) Size() (int, int)        { return i.w, i.h }
func (i *Image) ColorModel() color.Model { return color.RGBAModel }
func (i *Image) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= i.w || y >= i.h {
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

// ColorScale is a stub for Ebiten's ColorScale.
type ColorScale struct{}

// Scale is a no-op in the stub.
func (c *ColorScale) Scale(r, g, b, a float32) {}

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
)

// Window and run stubs
type Game interface {
	Update() error
	Draw(*Image)
	Layout(int, int) (int, int)
}

const SyncWithFPS = -1

func SetWindowSize(w, h int)      {}
func SetWindowTitle(title string) {}
func SetTPS(tps int)              {}
func RunGame(g Game) error        { return nil }

// CursorShapeType represents a shape of a mouse cursor.
type CursorShapeType int

const (
	CursorShapeDefault    CursorShapeType = iota
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
