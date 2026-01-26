package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// ColorWheelProps contains the external state passed to the color wheel component.
type ColorWheelProps struct {
	// AnchorRect is the button that opens the wheel (used for positioning).
	AnchorRect image.Rectangle
	// Bounds is the container bounds for clamping the wheel position.
	Bounds image.Rectangle
	// RowHeight is used for calculating wheel size.
	RowHeight int
	// OnColorPick is called when a color is picked.
	OnColorPick func(c color.Color)
	// OnClose is called when the wheel should close.
	OnClose func()
}

// ColorWheelState contains the internal state for the color wheel component.
type ColorWheelState struct {
	open     bool
	hold     bool // Capture flag (set when opened, released on mouse-up)
	wheelImg *ebiten.Image
	cacheW   int
	cacheH   int
}

// ColorWheelComponent is a self-contained HSV color picker wheel.
type ColorWheelComponent struct {
	BaseComponent
	props ColorWheelProps
	state ColorWheelState
}

// NewColorWheelComponent creates a new color wheel component.
func NewColorWheelComponent(id string) *ColorWheelComponent {
	return &ColorWheelComponent{
		BaseComponent: *NewBaseComponent(id),
	}
}

// SetProps updates the external props.
func (c *ColorWheelComponent) SetProps(p ColorWheelProps) {
	c.props = p
}

// Props returns the current props.
func (c *ColorWheelComponent) Props() ColorWheelProps { return c.props }

// Open opens the color wheel.
func (c *ColorWheelComponent) Open() {
	c.state.open = true
	c.state.hold = true
	c.rebuildWheel()
}

// Close closes the color wheel.
func (c *ColorWheelComponent) Close() {
	c.state.open = false
	c.state.hold = false
	if c.props.OnClose != nil {
		c.props.OnClose()
	}
}

// IsOpen returns whether the color wheel is currently open.
func (c *ColorWheelComponent) IsOpen() bool {
	return c.state.open
}

// rebuildWheel recalculates wheel position and regenerates the image cache.
func (c *ColorWheelComponent) rebuildWheel() {
	if c.props.AnchorRect.Empty() || c.props.Bounds.Empty() {
		c.SetBounds(image.Rectangle{})
		return
	}

	base := c.props.AnchorRect
	bounds := c.props.Bounds
	rowH := c.props.RowHeight
	if rowH <= 0 {
		rowH = 24
	}

	// Calculate wheel size
	target := rowH * 6
	if target < 60 {
		target = 60
	}
	if target > 200 {
		target = 200
	}
	maxSize := bounds.Dx()
	if bounds.Dy() < maxSize {
		maxSize = bounds.Dy()
	}
	if maxSize < 1 {
		maxSize = 1
	}
	wheel := target
	if wheel > maxSize {
		wheel = maxSize
	}
	if wheel < 20 {
		wheel = maxSize
	}

	// Position: prefer above anchor, else below; clamp to bounds
	wantY := base.Min.Y - wheel
	if wantY < bounds.Min.Y {
		wantY = base.Max.Y
	}
	if wantY < bounds.Min.Y {
		wantY = bounds.Min.Y
	}
	if wantY > bounds.Max.Y-wheel {
		wantY = bounds.Max.Y - wheel
	}
	if wantY < bounds.Min.Y {
		wantY = bounds.Min.Y
	}

	wantX := base.Min.X
	if wantX < bounds.Min.X {
		wantX = bounds.Min.X
	}
	if wantX > bounds.Max.X-wheel {
		wantX = bounds.Max.X - wheel
	}
	if wantX < bounds.Min.X {
		wantX = bounds.Min.X
	}

	rect := image.Rect(wantX, wantY, wantX+wheel, wantY+wheel)
	c.SetBounds(rect)
	c.rebuildImage()
}

// rebuildImage regenerates the cached wheel image.
func (c *ColorWheelComponent) rebuildImage() {
	r := c.bounds
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		c.state.wheelImg = nil
		c.state.cacheW, c.state.cacheH = 0, 0
		return
	}

	if w == c.state.cacheW && h == c.state.cacheH && c.state.wheelImg != nil {
		return // cache is valid
	}

	buf := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			col := c.pickColorAt(r.Min.X+x, r.Min.Y+y)
			rr, gg, bb, aa := color.RGBAModel.Convert(col).(color.RGBA).RGBA()
			buf.SetRGBA(x, y, color.RGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), uint8(aa >> 8)})
		}
	}
	c.state.wheelImg = ebiten.NewImageFromImage(buf)
	c.state.cacheW, c.state.cacheH = w, h
}

// pickColorAt maps a screen coordinate to a color using HSV.
func (c *ColorWheelComponent) pickColorAt(x, y int) color.Color {
	r := c.bounds
	if r.Empty() {
		return color.RGBA{200, 200, 200, 255}
	}

	cx := float64(r.Min.X + r.Dx()/2)
	cy := float64(r.Min.Y + r.Dy()/2)
	rx := float64(x) - cx
	ry := float64(y) - cy
	radius := float64(imin(r.Dx(), r.Dy())) / 2
	if radius <= 0 {
		return color.RGBA{200, 200, 200, 255}
	}

	rnorm := math.Hypot(rx, ry) / radius
	if rnorm > 1 {
		rnorm = 1
	}

	h := math.Atan2(ry, rx)
	if h < 0 {
		h += 2 * math.Pi
	}
	h /= 2 * math.Pi

	// Two-zone mapping for broader gamut:
	// Inner half: saturated darks (s=1, v in [0..1])
	// Outer half: bright pastels to saturated (v=1, s in [0..1])
	var s, v float64
	if rnorm < 0.5 {
		s = 1
		v = rnorm / 0.5
	} else {
		s = (rnorm - 0.5) / 0.5
		v = 1
	}
	return colorWheelHSVToRGBA(h, s, v)
}

// colorWheelHSVToRGBA converts HSV to RGBA (local to avoid dependency on DrumView).
func colorWheelHSVToRGBA(h, s, v float64) color.Color {
	if s <= 0 {
		c := uint8(clamp(int(v*255), 0, 255))
		return color.RGBA{c, c, c, 255}
	}
	h6 := h * 6
	i := int(math.Floor(h6))
	f := h6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{
		uint8(clamp(int(r*255), 0, 255)),
		uint8(clamp(int(g*255), 0, 255)),
		uint8(clamp(int(b*255), 0, 255)),
		255,
	}
}

// HandleInput processes mouse input for the color wheel.
func (c *ColorWheelComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.state.open {
		return InputIgnored
	}

	// If hold is active (just opened), capture until release
	if c.state.hold {
		if !pressed {
			c.state.hold = false
		}
		return InputCaptured
	}

	pt := image.Pt(x, y)

	// If pressed inside wheel, pick color
	if pressed && pt.In(c.bounds) {
		col := c.pickColorAt(x, y)
		if c.props.OnColorPick != nil {
			c.props.OnColorPick(col)
		}
		c.Close()
		SuppressClicksUntilMouseUp()
		return InputConsumed
	}

	// If pressed outside wheel, close
	if pressed && !pt.In(c.bounds) {
		c.Close()
		SuppressClicksUntilMouseUp()
		return InputConsumed
	}

	// If within bounds but not pressed, consume to prevent click-through
	if pt.In(c.bounds) {
		return InputConsumed
	}

	return InputIgnored
}

// Draw renders the color wheel.
func (c *ColorWheelComponent) Draw(dst *ebiten.Image) {
	if !c.state.open {
		return
	}

	r := c.bounds
	if r.Empty() {
		return
	}

	// Check if cache needs rebuild
	if c.state.wheelImg == nil || c.state.cacheW != r.Dx() || c.state.cacheH != r.Dy() {
		c.rebuildImage()
	}

	if c.state.wheelImg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(c.state.wheelImg, op)
	}

	// Draw border
	drawRect(dst, r, colButtonBorder, false)
}

// Capturing returns whether the component is capturing input.
func (c *ColorWheelComponent) Capturing() bool {
	return c.state.hold
}

// InputBounds returns the wheel bounds for overlay compatibility.
func (c *ColorWheelComponent) InputBounds() image.Rectangle {
	if !c.state.open {
		return image.Rectangle{}
	}
	return c.bounds
}

// WheelRect returns the current wheel rectangle (for testing/debugging).
func (c *ColorWheelComponent) WheelRect() image.Rectangle {
	return c.bounds
}

// HandleWheel consumes wheel events to prevent pass-through.
func (c *ColorWheelComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.state.open {
		return InputIgnored
	}
	return InputConsumed
}
