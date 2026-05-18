package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// NotificationsLayer paints up to 2 active toast notifications at the
// top-right of the drum view. Decays each notification's ttl and prunes
// expired entries. Migrated verbatim from the former DrumView.drawNotifications.
//
// Z = ZNotifications (170) — above transport pulse / view switch /
// row-zoom chips, below debug overlays.
type NotificationsLayer struct {
	dv *DrumView
}

func newNotificationsLayer(dv *DrumView) *NotificationsLayer { return &NotificationsLayer{dv: dv} }

func (l *NotificationsLayer) ID() string    { return "notifications" }
func (l *NotificationsLayer) ZIndex() int   { return ZNotifications }
func (l *NotificationsLayer) Visible() bool { return !l.dv.simpleDraw }

func (l *NotificationsLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	out := dv.notifs[:0]
	for i := range dv.notifs {
		n := dv.notifs[i]
		if n.ttl <= 0 {
			continue
		}
		n.ttl--
		out = append(out, n)
	}
	dv.notifs = out
	maxShow := 2
	pad := 6
	y := dv.Bounds.Min.Y + 6
	for i := len(dv.notifs) - 1; i >= 0 && maxShow > 0; i-- {
		n := dv.notifs[i]
		txt := n.msg
		w := TextWidth(txt)
		h := TextHeight() + pad
		boxW := w + pad*2
		x := dv.Bounds.Max.X - boxW - 10
		r := image.Rect(x, y, x+boxW, y+h)
		fill := colBPMBox
		if n.isErr {
			fill = colError
		}
		drawButton(dst, r, fill, colButtonBorder, false, Profile().DrawTopEdgeHighlight)
		DrawTextAt(dst, txt, r.Min.X+pad, r.Min.Y+(h-TextHeight())/2)
		y += h + 4
		maxShow--
	}
}
