package ui

// Drag adjusts live per detent and commits ONE undo step on release; a ±1 magnet at the press reading writes
// the start values back exactly. Tap opens the value wheel; the mouse wheel scrolls the grid, never edits.

import (
	"image"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// synthDialDragSpanPx is the pointer travel that sweeps a dial end to end.
const synthDialDragSpanPx = 200

const synthDialMagnet = 1

// synthDialByID finds a card's resolved dial on the active instrument.
func (dv *DrumView) synthDialByID(card, dial string) (synthdial.Resolved, bool) {
	cg := dv.macroConceptByID(card)
	if cg == nil {
		return synthdial.Resolved{}, false
	}
	for _, d := range cg.Dials {
		if d.Dial.ID == dial {
			return d, true
		}
	}
	return synthdial.Resolved{}, false
}

// synthDialHitAdapter is one dial's input handler.
type synthDialHitAdapter struct {
	dv     *DrumView
	instID instrument.ID
	card   string
	dial   string
	rect   image.Rectangle
	startY int
	moved  bool
	g      *synthDialGesture
}

func (h *synthDialHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

func (h *synthDialHitAdapter) resolved() instrument.ID { return h.dv.resolveSynthInstrument(h.instID) }

func (h *synthDialHitAdapter) OnPress(x, y int) InputResult {
	if h.dv == nil {
		return InputIgnored
	}
	g, ok := h.dv.beginSynthDialGesture(h.resolved(), h.card, h.dial)
	if !ok {
		return InputIgnored
	}
	h.dv.setSynthFocusDial(h.resolved(), h.card, h.dial)
	h.g, h.startY, h.moved = g, y, false
	return InputCaptured
}

// Up is positive.
func synthDialDragTarget(start, dy int) int {
	t := start + dy*100/synthDialDragSpanPx
	if d := t - start; d <= synthDialMagnet && d >= -synthDialMagnet {
		return start
	}
	return max(0, min(100, t))
}

func (h *synthDialHitAdapter) OnDrag(x, y int) {
	if h.g == nil {
		return
	}
	if !h.moved && abs(y-h.startY) < tapMaxMovePx {
		return
	}
	h.moved = true
	before := h.g.last
	h.dv.turnSynthDial(h.g, synthDialDragTarget(h.g.startReading, h.startY-y))
	if h.g.last != before {
		h.dv.invalidateSynthPanel()
	}
}

func (h *synthDialHitAdapter) OnRelease(x, y int) {
	g := h.g
	if g == nil {
		return
	}
	h.g = nil
	if h.moved {
		h.dv.endSynthDialGesture(g, true)
		h.dv.invalidateSynthPanel()
		return
	}
	h.dv.endSynthDialGesture(g, false)
	h.dv.openSynthDialWheel(g.inst, h.card, h.dial, h.rect)
}

func (h *synthDialHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// invalidateSynthPanel re-lays out the audio panel so the cards repaint.
func (dv *DrumView) invalidateSynthPanel() {
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
	}
}

// synthDialWheelLadder is the dial wheel's step ladder (fine → coarse).
var synthDialWheelLadder = []float64{1, 5, 10, 25}

// One gesture: live per detent, one commit on Accept, the opening values verbatim on Esc.
func (dv *DrumView) openSynthDialWheel(inst instrument.ID, card, dial string, anchor image.Rectangle) {
	d, ok := dv.synthDialByID(card, dial)
	if !ok || inst == "" || len(d.Terms) == 0 {
		return
	}
	// Not synthDialActive: an untouched wheel closes without OnCommit/OnCancel and could never clear the freeze.
	dv.setSynthFocusDial(inst, card, dial)
	g := newSynthDialGesture(inst, card, d)
	m := &NumberModel{
		Min: 0, Max: 100, Value: float64(g.startReading),
		Ladder:       synthDialWheelLadder,
		Format:       func(v, _ float64) string { return strconv.Itoa(int(v)) },
		LadderFormat: func(step float64) string { return "±" + strconv.Itoa(int(step)) },
	}
	// Title only: a dial's one-liner does not fit the wheel's width.
	label := i18n.T(i18n.Key(g.d.Dial.Label))
	dv.openValueWheel(WheelBinding{
		Model: m,
		Title: func() string { return label },
		OnChange: func() {
			dv.turnSynthDial(g, int(m.Value+0.5))
			dv.invalidateSynthPanel()
		},
		OnCancel: func() {
			dv.endSynthDialGesture(g, false)
			dv.invalidateSynthPanel()
		},
		OnCommit: func() {
			dv.endSynthDialGesture(g, true)
			dv.invalidateSynthPanel()
		},
	}, anchor)
}
