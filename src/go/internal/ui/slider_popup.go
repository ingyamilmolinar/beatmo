package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// popupThumbSize is the thumb diameter used for slider popup rendering.
const popupThumbSize = 12

// SliderPopupConfig configures a reusable vertical slider popup.
type SliderPopupConfig struct {
	ID       string
	ZIndex   int
	GetValue func() float64
	SetValue func(float64)
	Label    func() string // optional label above percentage (e.g. frequency); nil = no label
	OnClose  func()        // optional callback on close
}

// SliderPopup is a reusable vertical slider popup panel.
type SliderPopup struct {
	config   SliderPopupConfig
	open     bool
	rect     image.Rectangle
	dragging bool
}

// NewSliderPopup creates a new slider popup with the given config.
func NewSliderPopup(cfg SliderPopupConfig) *SliderPopup {
	return &SliderPopup{config: cfg}
}

// Open positions and opens the popup above (or below) the anchor rect,
// clamped within bounds. headerH is the header height to avoid.
func (sp *SliderPopup) Open(anchor, bounds image.Rectangle, headerH int) {
	popupW, popupH := 52, 160

	x := anchor.Min.X + anchor.Dx()/2 - popupW/2
	y := anchor.Min.Y - popupH - 8
	if y < bounds.Min.Y+headerH {
		y = anchor.Max.Y + 8
	}
	if x < bounds.Min.X+4 {
		x = bounds.Min.X + 4
	}
	if x+popupW > bounds.Max.X-4 {
		x = bounds.Max.X - 4 - popupW
	}

	sp.rect = image.Rect(x, y, x+popupW, y+popupH)
	sp.dragging = false
	sp.open = true
}

// Close closes the popup.
func (sp *SliderPopup) Close() {
	sp.open = false
	sp.dragging = false
	if sp.config.OnClose != nil {
		sp.config.OnClose()
	}
}

// IsOpen returns whether the popup is currently open.
func (sp *SliderPopup) IsOpen() bool { return sp.open }

// Rect returns the popup panel rect.
func (sp *SliderPopup) Rect() image.Rectangle { return sp.rect }

// IsDragging returns whether the user is currently dragging the thumb.
func (sp *SliderPopup) IsDragging() bool { return sp.dragging }

// trackTop returns the Y coordinate where the vertical track begins.
func (sp *SliderPopup) trackTop() int {
	if sp.config.Label != nil {
		return sp.rect.Min.Y + 32
	}
	return sp.rect.Min.Y + 28
}

// trackBot returns the Y coordinate where the vertical track ends.
func (sp *SliderPopup) trackBot() int {
	return sp.rect.Max.Y - 8
}

// Draw renders the popup panel with optional label, percentage, track, and thumb.
func (sp *SliderPopup) Draw(dst *ebiten.Image) {
	if !sp.open || sp.rect.Empty() {
		return
	}
	r := sp.rect
	drawPanel(dst, r)

	val := sp.config.GetValue()

	yOff := 0
	// Optional label (e.g. frequency).
	if sp.config.Label != nil {
		label := sp.config.Label()
		lx := r.Min.X + (r.Dx()-len(label)*debugCharW)/2
		ly := r.Min.Y + 4
		DrawTextAt(dst, label, lx, ly)
		yOff = 12
	}

	// Percentage text.
	pct := int(math.Round(val * 100))
	pctLabel := fmt.Sprintf("%d%%", pct)
	px := r.Min.X + (r.Dx()-len(pctLabel)*debugCharW)/2
	py := r.Min.Y + 6 + yOff
	DrawTextAt(dst, pctLabel, px, py)

	// Vertical track.
	trackX := r.Min.X + r.Dx()/2 - 1
	trackTop := sp.trackTop()
	trackBot := sp.trackBot()
	trackH := trackBot - trackTop
	if trackH <= 0 {
		return
	}
	trackRect := image.Rect(trackX, trackTop, trackX+2, trackBot)
	drawRect(dst, trackRect, colSubtleBorder, true)

	// Filled portion.
	thumbY := trackTop + int(float64(trackH)*(1-val))
	if thumbY < trackTop {
		thumbY = trackTop
	}
	if thumbY > trackBot {
		thumbY = trackBot
	}
	if thumbY < trackBot {
		drawRect(dst, image.Rect(trackX, thumbY, trackX+2, trackBot), colStep, true)
	}

	// Thumb.
	thumbRect := image.Rect(
		trackX-popupThumbSize/2+1,
		thumbY-popupThumbSize/2,
		trackX+popupThumbSize/2+1,
		thumbY+popupThumbSize/2,
	)
	drawRect(dst, thumbRect, colStep, true)
	drawRect(dst, thumbRect, color.RGBA{255, 255, 255, 180}, false)
}

// HandleInput processes mouse/touch interaction with the popup.
// Returns true if the popup consumed the input.
func (sp *SliderPopup) HandleInput(mx, my int, pressed bool) bool {
	if !sp.open {
		return false
	}

	trackTop := sp.trackTop()
	trackBot := sp.trackBot()
	trackH := trackBot - trackTop
	if trackH <= 0 {
		return false
	}

	if pressed && (sp.dragging || image.Pt(mx, my).In(sp.rect)) {
		sp.dragging = true
		clampedY := my
		if clampedY < trackTop {
			clampedY = trackTop
		}
		if clampedY > trackBot {
			clampedY = trackBot
		}
		val := 1.0 - float64(clampedY-trackTop)/float64(trackH)
		val = math.Round(val*100) / 100
		if val < 0 {
			val = 0
		}
		if val > 1 {
			val = 1
		}
		sp.config.SetValue(val)
		return true
	}
	if !pressed {
		sp.dragging = false
	}
	return false
}

// SliderPopupOverlay wraps a SliderPopup as a generic Overlay.
type SliderPopupOverlay struct {
	Popup *SliderPopup
}

func (o *SliderPopupOverlay) ID() string                   { return o.Popup.config.ID }
func (o *SliderPopupOverlay) IsOpen() bool                 { return o.Popup.IsOpen() }
func (o *SliderPopupOverlay) ZIndex() int                  { return o.Popup.config.ZIndex }
func (o *SliderPopupOverlay) InputBounds() image.Rectangle { return o.Popup.Rect() }
func (o *SliderPopupOverlay) Capturing() bool              { return o.Popup.IsDragging() }
func (o *SliderPopupOverlay) Close() {
	o.Popup.Close()
}
func (o *SliderPopupOverlay) HandleInput(x, y int, pressed bool) InputResult {
	if o.Popup.HandleInput(x, y, pressed) {
		if o.Capturing() {
			return InputCaptured
		}
		return InputConsumed
	}
	return InputIgnored
}
func (o *SliderPopupOverlay) HandleWheel(x, y, steps int) InputResult {
	return InputConsumed // prevent scroll-through
}
