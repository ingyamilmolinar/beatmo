package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// logsToastTTLFrames is how long an error toast stays visible (~4s at 60fps)
// before it disappears. Errors are rare, so a fixed dwell is fine; the full
// message also lives in the Logs panel.
const logsToastTTLFrames = 240

// logsToast is a minimal transient alert shown at the top of the grid pane for
// ERROR notifications only (info events accumulate quietly in the badge/panel).
// It replaces the removed in-band notification area's alert role.
type logsToast struct {
	text   string
	frames int // remaining visible frames; 0 = inactive
}

func (t *logsToast) active() bool { return t.frames > 0 }

// dismiss hides the toast immediately (used by "Clear history").
func (t *logsToast) dismiss() { t.frames = 0; t.text = "" }

// push shows the toast with the given text for the full TTL.
func (t *logsToast) push(text string) {
	t.text = text
	t.frames = logsToastTTLFrames
}

// tick counts down one frame; call once per frame.
func (t *logsToast) tick() {
	if t.frames > 0 {
		t.frames--
	}
}

// rectIn returns the toast's on-screen rect within the grid pane (empty when
// inactive). The toast is a passive alert; the always-present logs icon (with
// its unread badge) is the affordance for opening the panel.
func (t *logsToast) rectIn(gridRect image.Rectangle) image.Rectangle {
	if !t.active() || t.text == "" {
		return image.Rectangle{}
	}
	w := TextWidth(t.text) + 2*SpaceMD
	h := TextHeight() + 2*SpaceSM
	cx := gridRect.Min.X + gridRect.Dx()/2
	top := gridRect.Min.Y + SpaceMD
	return image.Rect(cx-w/2, top, cx+w/2, top+h)
}

// draw paints the toast in error red at the top-center of the grid pane.
func (t *logsToast) draw(dst *ebiten.Image, gridRect image.Rectangle) {
	r := t.rectIn(gridRect)
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, colSurface3, RadiusMD, true)
	drawRoundedRect(dst, r, colError, RadiusMD, false)
	DrawTextColorAt(dst, t.text, r.Min.X+SpaceMD, r.Min.Y+SpaceSM, colError)
}
