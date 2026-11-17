package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// BackgroundLayer paints the drum-pane background and the per-widget
// surface fills. On mobile it also paints the bottom-action-bar sheet
// surface so the toolbar buttons hosted in the bar render on top of it.
//
// Z = ZBackground (50) — first thing drawn into the drum pane. Every
// other layer composites on top.
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
		// The drum pane is ONE bed (rack-bed, Background category): the rack,
		// the area under the rows and the header row — transport strip and
		// the timeline header band right of the beat counter — all share it.
		// The header used to take the base background, a black band running
		// the width of the pane above the lighter rack.
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

	// Mobile bottom action bar sheet surface. The transport widget's
	// clip rect at the top excludes this rect, so it must be painted
	// here (before the transport zone, so vol/view/overflow buttons
	// hosted in the bar render on top of the surface).
	if !dv.bottomActionBarRect.Empty() {
		drawBottomSheetPanel(dst, dv.bottomActionBarRect)
	}
}
