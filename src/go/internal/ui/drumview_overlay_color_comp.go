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
	picked   bool // Set when a color is picked; defers Close() until mouse release
	wheelImg *ebiten.Image
	cacheW   int
	cacheH   int
}

// ColorWheelComponent is a self-contained HSV color picker wheel.
type ColorWheelComponent struct {
	overlayBase
	props    ColorWheelProps
	state    ColorWheelState
	closeBtn *Button
}

// NewColorWheelComponent creates a new color wheel component.
func NewColorWheelComponent() *ColorWheelComponent {
	return &ColorWheelComponent{
		overlayBase: newOverlayBase(),
	}
}

// SetProps updates the external props.
func (c *ColorWheelComponent) SetProps(p ColorWheelProps) {
	c.props = p
}

// Props returns the current props.
func (c *ColorWheelComponent) Props() ColorWheelProps { return c.props }

// Open opens the color wheel.
// Note: hold is NOT set here because the portal tree's capture system
// prevents double-dispatch of the opening press. Setting hold would block
// all subsequent input until a mouse release clears it.
func (c *ColorWheelComponent) Open() {
	c.state.open = true
	c.rebuildWheel()
}

// ClearHold clears the hold state (for testing when opened via direct callback).
func (c *ColorWheelComponent) ClearHold() {
	c.state.hold = false
}

// Close closes the color wheel.
func (c *ColorWheelComponent) Close() {
	c.state.open = false
	c.state.hold = false
	c.state.picked = false
	if c.props.OnClose != nil {
		c.props.OnClose()
	}
}

// IsOpen returns whether the color wheel is currently open.
func (c *ColorWheelComponent) IsOpen() bool {
	return c.state.open
}

// rebuildWheel recalculates wheel position and regenerates the image cache.
// The wheel fills the smaller dimension of Bounds (square) and is centered.
func (c *ColorWheelComponent) rebuildWheel() {
	if c.props.AnchorRect.Empty() || c.props.Bounds.Empty() {
		c.SetBounds(image.Rectangle{})
		return
	}

	bounds := c.props.Bounds

	// Diameter = smaller of container width/height (fill the section).
	wheel := bounds.Dx()
	if bounds.Dy() < wheel {
		wheel = bounds.Dy()
	}
	if wheel < 1 {
		wheel = 1
	}

	// Center within bounds.
	cx := bounds.Min.X + bounds.Dx()/2
	cy := bounds.Min.Y + bounds.Dy()/2
	half := wheel / 2
	rect := image.Rect(cx-half, cy-half, cx-half+wheel, cy-half+wheel)

	// Clamp to stay fully inside bounds.
	if rect.Min.X < bounds.Min.X {
		rect = rect.Add(image.Pt(bounds.Min.X-rect.Min.X, 0))
	}
	if rect.Min.Y < bounds.Min.Y {
		rect = rect.Add(image.Pt(0, bounds.Min.Y-rect.Min.Y))
	}
	if rect.Max.X > bounds.Max.X {
		rect = rect.Add(image.Pt(bounds.Max.X-rect.Max.X, 0))
	}
	if rect.Max.Y > bounds.Max.Y {
		rect = rect.Add(image.Pt(0, bounds.Max.Y-rect.Max.Y))
	}

	c.SetBounds(rect)
	c.rebuildImage()

	// Close button at top-right of wheel
	cr := closeButtonRect(rect, 2)
	c.closeBtn = NewButton("", PopupButtonStyle, func() { c.Close() })
	c.closeBtn.Icon = "close"
	c.closeBtn.IconColor = colButtonBorder
	c.closeBtn.SetRect(cr)
	c.closeBtn.ConsumeOnPress = true
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
		return genColorRowRackColorFallback
	}

	cx := float64(r.Min.X + r.Dx()/2)
	cy := float64(r.Min.Y + r.Dy()/2)
	rx := float64(x) - cx
	ry := float64(y) - cy
	radius := float64(imin(r.Dx(), r.Dy())) / 2
	if radius <= 0 {
		return genColorRowRackColorFallback
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

	// Deferred close after color pick: stay open until mouse release
	if c.state.picked {
		if !pressed {
			c.Close()
			return InputConsumed
		}
		return InputCaptured
	}

	// Close button (highest z-order)
	if c.closeBtn != nil && c.closeBtn.Handle(x, y, pressed) {
		return InputConsumed
	}

	pt := image.Pt(x, y)

	// If pressed inside wheel bounds, check if inside the circular wheel.
	if pressed && pt.In(c.bounds) {
		r := c.bounds
		cx := float64(r.Min.X + r.Dx()/2)
		cy := float64(r.Min.Y + r.Dy()/2)
		radius := float64(imin(r.Dx(), r.Dy())) / 2
		dx, dy := float64(x)-cx, float64(y)-cy
		if dx*dx+dy*dy > radius*radius {
			return InputConsumed // inside rect but outside circle — absorb without picking
		}
		col := c.pickColorAt(x, y)
		if c.props.OnColorPick != nil {
			c.props.OnColorPick(col)
		}
		c.state.picked = true
		return InputCaptured
	}

	// If pressed outside wheel, close
	if pressed && !pt.In(c.bounds) {
		c.Close()
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
	// Close button on top
	if c.closeBtn != nil {
		c.closeBtn.Draw(dst)
	}
}

// CloseBtn returns the close button (for testing).
func (c *ColorWheelComponent) CloseBtn() *Button { return c.closeBtn }

// Capturing returns whether the component is capturing input.
func (c *ColorWheelComponent) Capturing() bool {
	return c.state.hold || c.state.picked
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
