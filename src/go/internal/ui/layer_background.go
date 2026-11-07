package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// On mobile it also paints the bottom action bar's sheet, so the buttons in it render on top.
type BackgroundLayer struct {
	dv *DrumView
}

func newBackgroundLayer(dv *DrumView) *BackgroundLayer { return &BackgroundLayer{dv: dv} }

func (l *BackgroundLayer) ID() string    { return "background" }
func (l *BackgroundLayer) ZIndex() int   { return ZBackground }
func (l *BackgroundLayer) Visible() bool { return true }

func (l *BackgroundLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(dv.Bounds.Min.X), float64(dv.Bounds.Min.Y))
	dst.DrawImage(dv.bg(dv.Bounds.Dx(), dv.Bounds.Dy()), op)

	mobile := Profile().IsMobile()
	for _, kind := range []WidgetKind{WidgetTransport, WidgetRack, WidgetTimeline} {
		r := dv.widgetRects[kind]
		if r.Empty() {
			continue
		}
		// The drum pane is ONE bed (rack-bed): rack, under-rows area and header share it, so no dark band runs above
		// the rack.
		fill := colRackSurface
		if mobile && kind == WidgetTransport {
			fill = colTransportSurface
		}
		drawRect(dst, r, fill, true)
		if mobile && kind == WidgetTransport {
			drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
				WithAlpha(genColorBorder, genAlphaRowRackZebra), true)
		}
	}
	if r := dv.widgetRects[WidgetWave]; !r.Empty() {
		drawRect(dst, r, colEQBg, true)
	}

	// The transport's clip excludes this rect, so it is painted here, before the buttons hosted in the bar.
	if !dv.bottomActionBarRect.Empty() {
		drawBottomSheetPanel(dst, dv.bottomActionBarRect)
	}
}
