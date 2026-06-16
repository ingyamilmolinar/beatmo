package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synthOverflowSheet is a bottom-sheet overlay listing the Synth-tab
// header actions (Save / Save As / Reset) that the layout cascade had
// to drop because the header strip wasn't wide enough. Without this
// sheet a 360-px portrait phone would silently lose the Save button —
// the bug surfaced by the original audit screenshots.
//
// The sheet is built with the list of dropped action ids at the time
// of the cascade so a later resize doesn't surface a stale option set.
type synthOverflowSheet struct {
	dv       *DrumView
	resolved string
	actions  []string // "save", "save-as", "reset"
	rect     image.Rectangle
	rows     []image.Rectangle
	labels   []string
	closed   bool
}

func newSynthOverflowSheet(dv *DrumView, actions []string, resolved string) *synthOverflowSheet {
	labels := make([]string, 0, len(actions))
	for _, a := range actions {
		switch a {
		case "preview":
			labels = append(labels, "Preview")
		case "save":
			labels = append(labels, "Save")
		case "save-as":
			labels = append(labels, "Save As…")
		case "reset":
			labels = append(labels, "Reset")
		}
	}
	return &synthOverflowSheet{
		dv:       dv,
		resolved: resolved,
		actions:  append([]string(nil), actions...),
		labels:   labels,
	}
}

func (o *synthOverflowSheet) Close()            { o.closed = true }
func (o *synthOverflowSheet) ShouldClose() bool { return o.closed }

func (o *synthOverflowSheet) Layout(_ image.Rectangle, screenBounds image.Rectangle) {
	if len(o.actions) == 0 {
		o.rect = image.Rectangle{}
		o.rows = nil
		return
	}
	dv := Profile().DensityValues()
	rowH := dv.PopupBtnH
	if rowH < 36 {
		rowH = 36
	}
	pad := dv.PopupPad
	if pad < 4 {
		pad = 4
	}
	sheetH := rowH*len(o.actions) + pad*(len(o.actions)+1)
	sheetW := screenBounds.Dx() - pad*2
	if sheetW > 320 {
		sheetW = 320
	}
	x0 := screenBounds.Min.X + (screenBounds.Dx()-sheetW)/2
	y0 := screenBounds.Max.Y - sheetH - pad
	if y0 < screenBounds.Min.Y+pad {
		y0 = screenBounds.Min.Y + pad
	}
	o.rect = image.Rect(x0, y0, x0+sheetW, y0+sheetH)
	o.rows = o.rows[:0]
	for i := range o.actions {
		ry := y0 + pad + i*(rowH+pad)
		o.rows = append(o.rows, image.Rect(x0+pad, ry, x0+sheetW-pad, ry+rowH))
	}
}

func (o *synthOverflowSheet) HitAreas() []HitArea {
	areas := make([]HitArea, 0, len(o.rows))
	for i, r := range o.rows {
		if r.Empty() {
			continue
		}
		idx := i
		areas = append(areas, HitArea{
			Rect:    r,
			Handler: &synthOverflowSheetRowHandler{sheet: o, row: idx},
			Tag:     "synth-overflow-row",
			Touch:   true,
		})
	}
	return areas
}

func (o *synthOverflowSheet) Draw(screen *ebiten.Image) {
	if o.rect.Empty() {
		return
	}
	drawRoundedRect(screen, o.rect, colSurface2, RadiusMD, true)
	drawRoundedRect(screen, o.rect, colButtonBorder, RadiusMD, false)
	for i, r := range o.rows {
		if r.Empty() {
			continue
		}
		drawRoundedRect(screen, r, WithAlpha(colSurface3, AlphaSubtle), RadiusSM, true)
		DrawTextColorAt(screen, o.labels[i], r.Min.X+8, r.Min.Y+(r.Dy()-TextHeight())/2, colTextPrimary)
	}
}

// SynthOverflowRows returns the laid-out row rects in label order.
// Exposed so tests can verify the sheet is reachable + that an action
// fires against the live audio state.
func (o *synthOverflowSheet) SynthOverflowRows() []image.Rectangle {
	out := make([]image.Rectangle, len(o.rows))
	copy(out, o.rows)
	return out
}

// SynthOverflowLabels mirrors SynthOverflowRows for test assertions.
func (o *synthOverflowSheet) SynthOverflowLabels() []string {
	out := make([]string, len(o.labels))
	copy(out, o.labels)
	return out
}

// synthOverflowSheetRowHandler closes the sheet and fires the
// corresponding action against the DrumView.
type synthOverflowSheetRowHandler struct {
	sheet *synthOverflowSheet
	row   int
}

func (h *synthOverflowSheetRowHandler) OnPress(_, _ int) InputResult { return InputConsumed }
func (h *synthOverflowSheetRowHandler) OnDrag(_, _ int)              {}
func (h *synthOverflowSheetRowHandler) OnRelease(_, _ int) {
	if h.sheet == nil || h.sheet.dv == nil || h.row < 0 || h.row >= len(h.sheet.actions) {
		return
	}
	action := h.sheet.actions[h.row]
	switch action {
	case "preview":
		h.sheet.dv.previewActiveSynth()
	case "save":
		h.sheet.dv.SaveActiveRecipe()
	case "save-as":
		h.sheet.dv.openSaveAsDialog()
	case "reset":
		if h.sheet.resolved != "" {
			audio.ResetInstrumentParams(h.sheet.resolved)
		}
	}
	h.sheet.Close()
	if dv := h.sheet.dv; dv != nil && dv.audioTree != nil {
		dv.audioTree.Portal().Close("synth-overflow-sheet")
	}
}
func (h *synthOverflowSheetRowHandler) OnWheel(_, _, _ int) InputResult { return InputIgnored }

// openSynthOverflowSheet opens the bottom-sheet overlay for the given
// dropped actions. Called from the overflow chevron's OnClick.
func (dv *DrumView) openSynthOverflowSheet(actions []string, resolved string) {
	if dv == nil || dv.audioTree == nil || len(actions) == 0 {
		return
	}
	portal := dv.audioTree.Portal()
	if portal == nil {
		return
	}
	sheet := newSynthOverflowSheet(dv, actions, resolved)
	portal.Open(PortalEntry{
		ID:      "synth-overflow-sheet",
		Overlay: sheet,
		Modal:   true,
		Anchor:  dv.instEditorHeader.overflowRect,
	})
}
