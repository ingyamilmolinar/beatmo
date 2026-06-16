package ui

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// notifHistoryMaxRows bounds how many history rows the popup shows at once.
const notifHistoryMaxRows = 12

// IsNotifHistoryOpen reports whether the notification-history popup is open.
func (dv *DrumView) IsNotifHistoryOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("notif-history")
}

// openNotifHistoryPortal opens the static menu-like notification-history popup
// anchored under the in-band notification area. Clicking outside closes it
// (portal default for non-modal entries).
func (dv *DrumView) openNotifHistoryPortal() {
	if dv.tree == nil {
		return
	}
	if dv.IsNotifHistoryOpen() {
		dv.tree.Portal().Close("notif-history")
		return
	}
	dv.computeNotifHistoryRect()
	dv.tree.Portal().Open(PortalEntry{
		ID: "notif-history",
		Overlay: &dvOverlayPortal{
			id:       "notif-history",
			isOpenFn: dv.IsNotifHistoryOpen,
			rectFn:   func() image.Rectangle { return dv.notifHistoryRect },
			inputFn:  func(x, y int, pressed bool) InputResult { return InputConsumed },
			wheelFn:  func(x, y, steps int) InputResult { return InputConsumed },
			drawFn:   dv.drawNotifHistory,
		},
		Scrim: true,
	})
}

// computeNotifHistoryRect sizes and positions the popup below the notification
// area, clamped to the drum bounds.
func (dv *DrumView) computeNotifHistoryRect() {
	anchor := dv.notifRect
	rowH := TextHeight() + SpaceSM
	n := 0
	if dv.notifStore != nil {
		n = dv.notifStore.Len()
	}
	rows := n
	if rows > notifHistoryMaxRows {
		rows = notifHistoryMaxRows
	}
	if rows < 1 {
		rows = 1 // room for the "no notifications" placeholder
	}
	h := rows*rowH + 2*SpaceSM

	minW := 240
	w := anchor.Dx()
	if w < minW {
		w = minW
	}
	left := anchor.Min.X
	top := anchor.Max.Y + SpaceXS
	r := image.Rect(left, top, left+w, top+h)

	// Clamp into the drum bounds.
	if r.Max.X > dv.Bounds.Max.X-SpaceSM {
		shift := r.Max.X - (dv.Bounds.Max.X - SpaceSM)
		r = r.Sub(image.Pt(shift, 0))
	}
	if r.Min.X < dv.Bounds.Min.X+SpaceSM {
		r = r.Add(image.Pt(dv.Bounds.Min.X+SpaceSM-r.Min.X, 0))
	}
	if r.Max.Y > dv.Bounds.Max.Y-SpaceSM {
		r.Max.Y = dv.Bounds.Max.Y - SpaceSM
	}
	dv.notifHistoryRect = r
}

// drawNotifHistory paints the popup: a surface panel listing recent
// notifications newest-first, errors in red and info in the primary color.
func (dv *DrumView) drawNotifHistory(dst *ebiten.Image) {
	r := dv.notifHistoryRect
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, colSurface2, RadiusLG, true)
	drawRoundedRect(dst, r, colBorderSubtle, RadiusLG, false)

	clip := dst.SubImage(r).(*ebiten.Image)
	rowH := TextHeight() + SpaceSM
	x := r.Min.X + SpaceSM
	y := r.Min.Y + SpaceSM

	var hist []notification
	if dv.notifStore != nil {
		hist = dv.notifStore.History()
	}
	if len(hist) == 0 {
		DrawTextAt(clip, i18n.T(i18n.KeyNoNotifications), x, y)
		return
	}
	for i, n := range hist {
		if i >= notifHistoryMaxRows {
			break
		}
		col := colTextPrimary
		if n.isErr {
			col = colError
		}
		line := n.text
		if ts := formatNotifTime(n.unixMs); ts != "" {
			line = ts + "  " + n.text
		}
		DrawTextColorAt(clip, line, x, y, col)
		y += rowH
		if y > r.Max.Y-rowH {
			break
		}
	}
}

// formatNotifTime renders a unix-millis timestamp as HH:MM; empty when 0.
func formatNotifTime(unixMs int64) string {
	if unixMs <= 0 {
		return ""
	}
	return time.UnixMilli(unixMs).Format("15:04")
}
