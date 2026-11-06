package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logsToastTTLFrames is how long an ERROR toast stays visible (~4s at 60fps).
// Task toasts ignore it entirely — see sticky.
const logsToastTTLFrames = 240

// toastProgressBarH is the progress track height in pixels. Drawn with drawRect
// (not drawRoundedRect) so the collectFilledRects test interceptor can
// pixel-assert it: text drawn to a SubImage does NOT propagate to the parent
// under ebitenstub, so the bar is the only assertable evidence the toast
// rendered.
const toastProgressBarH = 4

// logsToast is a single-slot transient status surface at the top of the grid
// pane. It holds EITHER a transient error alert (plain text, TTL) OR a sticky
// determinate task (i18n key + done/total, no TTL, cleared by endTask).
//
// One slot on purpose: an error during a load replaces the progress line, which
// is the right priority. The full message of both lives on in the Logs panel.
type logsToast struct {
	text   string // error mode: frozen string
	frames int    // error mode: remaining visible frames; 0 = inactive

	sticky bool     // task mode: ignore the TTL
	key    i18n.Key // task mode: re-rendered every frame, so a locale switch mid-task is reflected
	done   int
	total  int
}

func (t *logsToast) active() bool { return t.sticky || t.frames > 0 }

// taskActive reports whether a sticky task (not an error) is showing.
func (t *logsToast) taskActive() bool { return t.sticky }

// dismiss hides the toast immediately (used by "Clear history").
func (t *logsToast) dismiss() { t.frames = 0; t.text = ""; t.sticky = false; t.key = "" }

// push shows an ERROR toast for the full TTL. An error takes the slot even from
// a running task — a failure is more urgent than progress.
func (t *logsToast) push(text string) {
	t.text = text
	t.frames = logsToastTTLFrames
	t.sticky = false
	t.key = ""
}

// beginTask starts a sticky determinate task. total <= 0 renders as
// indeterminate (label only, no bar).
func (t *logsToast) beginTask(key i18n.Key, total int) {
	t.key = key
	t.total = total
	t.done = 0
	t.sticky = true
	t.text = ""
	t.frames = 0
}

func (t *logsToast) updateTask(done int) {
	if !t.sticky {
		return
	}
	if done < 0 {
		done = 0
	}
	if t.total > 0 && done > t.total {
		done = t.total
	}
	t.done = done
}

func (t *logsToast) endTask() {
	if !t.sticky {
		return
	}
	t.sticky = false
	t.key = ""
	t.done, t.total = 0, 0
}

// tick counts down one frame; call once per frame. A sticky task never expires.
func (t *logsToast) tick() {
	if t.sticky {
		return
	}
	if t.frames > 0 {
		t.frames--
	}
}

// label renders the current line in the CURRENT locale. Task mode re-resolves
// the i18n key every call so a mid-task language switch is reflected; error
// mode returns its frozen string.
func (t *logsToast) label() string {
	if t.sticky && t.key != "" {
		return i18n.Tf(t.key, strconv.Itoa(t.done), strconv.Itoa(t.total))
	}
	return t.text
}

// progress returns the 0..1 fill fraction, and false when indeterminate.
func (t *logsToast) progress() (float64, bool) {
	if !t.sticky || t.total <= 0 {
		return 0, false
	}
	return float64(t.done) / float64(t.total), true
}

// rectIn returns the toast's on-screen rect within the grid pane (empty when
// inactive). The toast is passive: it registers no hit area and never takes
// input.
func (t *logsToast) rectIn(gridRect image.Rectangle) image.Rectangle {
	if !t.active() {
		return image.Rectangle{}
	}
	label := t.label()
	if label == "" {
		return image.Rectangle{}
	}
	w := TextWidth(label) + 2*SpaceMD
	h := TextHeight() + 2*SpaceSM
	if _, ok := t.progress(); ok {
		h += toastProgressBarH + SpaceSM
	}
	cx := gridRect.Min.X + gridRect.Dx()/2
	top := gridRect.Min.Y + SpaceMD
	return image.Rect(cx-w/2, top, cx+w/2, top+h)
}

// draw paints the toast at the top-centre of the supplied host rect.
func (t *logsToast) draw(dst *ebiten.Image, hostRect image.Rectangle) {
	r := t.rectIn(hostRect)
	if r.Empty() {
		return
	}
	accent := colError
	if t.sticky {
		accent = colAccent
	}
	rad := fitRadius(RadiusContainer, r)
	drawRoundedRect(dst, r, colSurface3, rad, true)
	drawRoundedRect(dst, r, accent, rad, false)

	textCol := colError
	if t.sticky {
		textCol = colTextPrimary
	}
	DrawTextColorAt(dst, t.label(), r.Min.X+SpaceMD, r.Min.Y+SpaceSM, textCol)

	if frac, ok := t.progress(); ok {
		bar := image.Rect(r.Min.X+SpaceMD, r.Max.Y-SpaceSM-toastProgressBarH,
			r.Max.X-SpaceMD, r.Max.Y-SpaceSM)
		// drawRect (not rounded): the pixel-test interceptor collects filled
		// rects, and this bar is the toast's only assertable evidence.
		drawRect(dst, bar, colSurface2, true)
		fillW := int(float64(bar.Dx()) * frac)
		if fillW > 0 {
			drawRect(dst, image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+fillW, bar.Max.Y), colAccent, true)
		}
	}
}
