//go:build test

package ebiten

import (
	"image"
	"image/color"
)

type Image struct{ w, h int }

func NewImage(w, h int) *Image { return &Image{w: w, h: h} }
func NewImageFromImage(img image.Image) *Image {
	b := img.Bounds()
	return &Image{w: b.Dx(), h: b.Dy()}
}

func (i *Image) DrawImage(src *Image, opts *DrawImageOptions) {}
func (i *Image) Fill(c color.Color)                           {}
func (i *Image) Bounds() image.Rectangle                      { return image.Rect(0, 0, i.w, i.h) }
func (i *Image) SubImage(r image.Rectangle) image.Image       { return &Image{w: r.Dx(), h: r.Dy()} }
func (i *Image) Size() (int, int)                             { return i.w, i.h }
func (i *Image) ColorModel() color.Model                      { return color.RGBAModel }
func (i *Image) At(x, y int) color.Color                      { return color.RGBA{} }

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

// Drawing options
type DrawImageOptions struct{ GeoM GeoM }

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
