package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synthSaveAsDialog is the inline modal of Save As, the only save (Synth and
// Sampler tabs alike): prefilled with the instrument's current name, its
// confirm button says what will happen as the user types — "Save over <name>"
// for the name of the library instrument being edited, "Save as new" for a new
// name — and another instrument's name is refused in place. The dialog lives
// inside the synth tab's content rect, blocks clicks behind it via its own
// HitArea (registered at ZSendPopover + 1), and is dismissed by OK / Cancel
// / Escape / tap-outside. The TextInput reuses the BPMBoxStyle chrome and
// soft-keyboard wiring already used by the WAV-import naming flow
// (drumview_portal_open.go), so mobile users get the same on-screen
// keyboard behaviour for free.
type synthSaveAsDialog struct {
	rect       image.Rectangle
	textInput  *TextInput
	okBtn      *Button
	cancelBtn  *Button
	instID     instrument.ID
	suggestion string
	// typed is the text classified last (classified once per change);
	// verdict is what confirming it would do.
	typed   string
	verdict saveAsVerdict
}

const (
	synthSaveAsDialogWidth  = 360
	synthSaveAsDialogHeight = 140
	synthSaveAsDialogMaxLen = 48
)

// saveAsMobileInputID is the native-input id of the Save As name box (Synth
// preset and Sampler sample share the dialog, never open at once).
const saveAsMobileInputID = "save-as-name"

// openSaveAsDialog opens Save As for the Synth tab's instrument.
func (dv *DrumView) openSaveAsDialog() { dv.openSaveAsDialogFor(dv.synthTabActiveInstrument()) }

// openSaveAsDialogFor opens the Save As dialog for instID, prefilled with the
// instrument's current name — the one the header and the row show. No-op
// without an instrument; idempotent, so a repeated call never loses
// in-progress text.
func (dv *DrumView) openSaveAsDialogFor(instID instrument.ID) {
	if dv == nil || instID == "" || dv.saveAsDialog != nil {
		return
	}
	suggestion := audio.InstrumentDisplayName(instID)
	if doc, ok := audio.InstrumentDoc(instID); ok && suggestion == "" {
		suggestion = doc.Name
	}
	dlg := &synthSaveAsDialog{instID: instID, suggestion: suggestion}
	dlg.layout(dv.eqPanelZone.ContentRect())
	dlg.textInput = NewTextField(dv.textFields, TextFieldSpec{ID: saveAsMobileInputID, Owner: "eq-panel"}, synthSaveAsDialogMaxLen)
	dlg.textInput.Rect = dlg.inputRect()
	dlg.textInput.SetText(suggestion)
	dlg.textInput.SetFocus(true)
	dlg.okBtn = NewSpecButton("", ComponentButtonPrimary, nil)
	dlg.cancelBtn = NewButtonKey(i18n.KeyCancel, ComponentButtonSecondary, nil)
	dlg.classify()
	dlg.cancelBtn.SetRect(dlg.cancelBtnRect())
	dv.saveAsDialog = dlg
}

// classify re-reads what confirming the typed name would do, when it changed,
// and labels the confirm button with it.
func (d *synthSaveAsDialog) classify() {
	typed := d.textInput.Value()
	if typed == d.typed && d.okBtn.Text != "" {
		return
	}
	d.typed = typed
	d.verdict = classifySaveAsName(typed, d.instID)
	d.okBtn.Text = d.verdict.label()
	d.okBtn.SetRect(d.okBtnRect())
}

// ConfirmSaveAsDialog runs the save the dialog shows and closes it. A refused
// (or empty) name keeps the dialog open. Public so the HitArea handler + tests
// + JS bridge drive it uniformly.
func (dv *DrumView) ConfirmSaveAsDialog() {
	if dv == nil || dv.saveAsDialog == nil {
		return
	}
	dlg := dv.saveAsDialog
	dlg.classify()
	if !dlg.verdict.ok() {
		return
	}
	dv.saveAsDialog = nil
	dv.saveAsInstrument(dlg.instID, strings.TrimSpace(dlg.typed))
}

// CancelSaveAsDialog closes the dialog without saving.
func (dv *DrumView) CancelSaveAsDialog() {
	if dv == nil {
		return
	}
	dv.saveAsDialog = nil
}

// CancelActiveTextDialog cancels + closes any open inline text-input dialog
// (Synth/Sampler "Save As"), discarding the typed value. Returns true if a
// dialog was open. This is the seam the universal Esc handler uses so Esc
// cancels the dialog instead of falling through to Stop. Portal-hosted text
// inputs (rename, instrument-menu search, WAV naming) are handled by the portal
// Esc path, not here.
func (dv *DrumView) CancelActiveTextDialog() bool {
	if dv == nil {
		return false
	}
	if dv.saveAsDialog != nil {
		dv.CancelSaveAsDialog()
		return true
	}
	return false
}

// updateSaveAsDialog polls the dialog's TextInput each frame and checks for
// Enter (confirm) / Escape (cancel). Called from DrumView.Update.
func (dv *DrumView) updateSaveAsDialog() {
	if dv == nil || dv.saveAsDialog == nil {
		return
	}
	dlg := dv.saveAsDialog
	// Sync rects in case the panel resized between frames.
	if dv.eqPanelZone != nil {
		dlg.layout(dv.eqPanelZone.ContentRect())
		dlg.textInput.Rect = dlg.inputRect()
		dlg.okBtn.SetRect(dlg.okBtnRect())
		dlg.cancelBtn.SetRect(dlg.cancelBtnRect())
	}
	// Mobile: the name is typed into the native <input>; mirror it so OK
	// confirms what the user sees.
	if v, ok := dlg.textInput.NativeLiveValue(); ok {
		dlg.textInput.SetText(v)
	}
	dlg.textInput.Update()
	dlg.classify()
	if isKeyPressed(ebiten.KeyEnter) {
		dv.ConfirmSaveAsDialog()
		return
	}
	// Esc cancel is owned by the universal handler (Game.handleEscape →
	// CancelActiveTextDialog), so it can take priority over the Stop fallback and
	// stay the single Esc authority. Do not re-handle Esc here.
}

// drawSaveAsDialog renders the dialog above the synth tab. Background
// panel chrome + title + TextInput + OK/Cancel buttons.
func (dv *DrumView) drawSaveAsDialog(dst *ebiten.Image) {
	if dv == nil || dv.saveAsDialog == nil {
		return
	}
	dlg := dv.saveAsDialog
	drawRoundedRect(dst, dlg.rect, TokenSurface2(), RadiusContainer, true)
	drawRoundedRect(dst, dlg.rect, TokenBorderMedium(), RadiusContainer, false)
	DrawTextStyled(dst, i18n.T(i18n.KeySaveAs),
		dlg.rect.Min.X+SpaceMD, dlg.rect.Min.Y+SpaceMD, RolePanelTitle, TokenTextPrimary())
	dlg.textInput.Draw(dst)
	if msg := dlg.Refused(); msg != "" {
		in := dlg.inputRect()
		DrawTextStyled(dst, msg, in.Min.X, in.Max.Y+SpaceXS, RoleCaption, colError)
	} else {
		dlg.okBtn.Draw(dst)
	}
	dlg.cancelBtn.Draw(dst)
}

// Focused reports whether the dialog's TextInput currently has focus
// (test-friendly accessor mirroring TextInput.Focused).
func (d *synthSaveAsDialog) Focused() bool {
	if d == nil || d.textInput == nil {
		return false
	}
	return d.textInput.Focused()
}

// ClaimsKeyboard reports whether the Save As dialog currently owns the
// keyboard. While its text input is focused, caret/typed keys belong to it, so
// the grid must not act on them. Part of the keyboard-ownership contract
// (keyboard_focus.go).
func (d *synthSaveAsDialog) ClaimsKeyboard() bool {
	return d != nil && d.textInput != nil && d.textInput.Focused()
}

// ConfirmLabel is what the confirm button says ("" while refused).
func (d *synthSaveAsDialog) ConfirmLabel() string {
	if d == nil || !d.verdict.ok() {
		return ""
	}
	return d.verdict.label()
}

// Refused is the in-place message for a name that belongs to another
// instrument ("" when the name can be saved).
func (d *synthSaveAsDialog) Refused() string {
	if d == nil || d.verdict.kind != saveAsTaken {
		return ""
	}
	return i18n.Tf(i18n.KeyNameTakenFmt, d.verdict.name)
}

// Value returns the current text. Test-friendly mirror of TextInput.Value.
func (d *synthSaveAsDialog) Value() string {
	if d == nil || d.textInput == nil {
		return ""
	}
	return d.textInput.Value()
}

// SetValue replaces the dialog's current text. Used by tests + the JS
// bridge to inject a typed string without driving the keyboard plumbing.
func (d *synthSaveAsDialog) SetValue(s string) {
	if d == nil || d.textInput == nil {
		return
	}
	d.textInput.SetText(s)
}

// Rect returns the dialog's bounding rectangle. Test accessor.
func (d *synthSaveAsDialog) Rect() image.Rectangle {
	if d == nil {
		return image.Rectangle{}
	}
	return d.rect
}

// OKRect / CancelRect expose button bounds so the browser test can click
// at the visible centers without depending on hit-area tag plumbing.
func (d *synthSaveAsDialog) OKRect() image.Rectangle {
	if d == nil || d.okBtn == nil {
		return image.Rectangle{}
	}
	return d.okBtn.Rect()
}

func (d *synthSaveAsDialog) CancelRect() image.Rectangle {
	if d == nil || d.cancelBtn == nil {
		return image.Rectangle{}
	}
	return d.cancelBtn.Rect()
}

// saveAsDialogHitAreas returns the OK / Cancel / frame hit areas for the open
// Save As dialog, or nil when closed. Shared by the Synth and Sampler tabs so
// the modal floats above either tab's content. OK/Cancel sit at zBase+1 so they
// win over the dialog frame (which consumes stray taps so a click inside the
// dialog but outside a control doesn't dismiss it).
func (dv *DrumView) saveAsDialogHitAreas(zBase int) []HitArea {
	if dv == nil || dv.saveAsDialog == nil {
		return nil
	}
	dlg := dv.saveAsDialog
	return []HitArea{
		{
			Rect:    dlg.OKRect(),
			ZIndex:  zBase + 1,
			Handler: &synthSaveAsDialogOKAdapter{dv: dv},
			Tag:     "save-as-dialog-ok",
			Touch:   true,
		},
		{
			Rect:    dlg.CancelRect(),
			ZIndex:  zBase + 1,
			Handler: &synthSaveAsDialogCancelAdapter{dv: dv},
			Tag:     "save-as-dialog-cancel",
			Touch:   true,
		},
		{
			Rect:    dlg.Rect(),
			ZIndex:  zBase,
			Handler: &synthSaveAsDialogFrameAdapter{dv: dv},
			Tag:     "save-as-dialog-frame",
		},
	}
}

func (d *synthSaveAsDialog) layout(contentR image.Rectangle) {
	if contentR.Empty() {
		return
	}
	w := synthSaveAsDialogWidth
	if w > contentR.Dx()-2*SpaceMD {
		w = contentR.Dx() - 2*SpaceMD
	}
	h := synthSaveAsDialogHeight
	if h > contentR.Dy()-2*SpaceMD {
		h = contentR.Dy() - 2*SpaceMD
	}
	cx := (contentR.Min.X + contentR.Max.X) / 2
	cy := (contentR.Min.Y + contentR.Max.Y) / 2
	d.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

func (d *synthSaveAsDialog) inputRect() image.Rectangle {
	inset := SpaceMD
	titleH := 20
	return image.Rect(
		d.rect.Min.X+inset,
		d.rect.Min.Y+inset+titleH+SpaceXS,
		d.rect.Max.X-inset,
		d.rect.Min.Y+inset+titleH+SpaceXS+28,
	)
}

// okBtnRect is as wide as its label ("Save over <name>" is long) and no
// wider than the space beside Cancel.
func (d *synthSaveAsDialog) okBtnRect() image.Rectangle {
	inset := SpaceMD
	btnH := 32
	btnW := 80
	if d.okBtn != nil {
		btnW = max(btnW, audioTabActionButtonWidth(d.okBtn.Text))
	}
	btnW = min(btnW, d.rect.Dx()-2*inset-80-SpaceSM)
	return image.Rect(
		d.rect.Max.X-inset-btnW,
		d.rect.Max.Y-inset-btnH,
		d.rect.Max.X-inset,
		d.rect.Max.Y-inset,
	)
}

// cancelBtnRect sits at the left edge, clear of the confirm button's width.
func (d *synthSaveAsDialog) cancelBtnRect() image.Rectangle {
	inset := SpaceMD
	btnW, btnH := 80, 32
	return image.Rect(
		d.rect.Min.X+inset,
		d.rect.Max.Y-inset-btnH,
		d.rect.Min.X+inset+btnW,
		d.rect.Max.Y-inset,
	)
}

// saveAsKind is what confirming a typed Save As name does.
type saveAsKind int

const (
	saveAsEmpty saveAsKind = iota // nothing typed: no save
	saveAsOver                    // the name of the instrument being edited: a new version of it
	saveAsNew                     // a new name: a new instrument
	saveAsTaken                   // another instrument's name: refused
)

type saveAsVerdict struct {
	kind saveAsKind
	name string // the instrument's name as shown (over / taken)
}

func (v saveAsVerdict) ok() bool { return v.kind == saveAsOver || v.kind == saveAsNew }

func (v saveAsVerdict) label() string {
	switch v.kind {
	case saveAsOver:
		return i18n.Tf(i18n.KeySaveOverFmt, v.name)
	case saveAsNew:
		return i18n.T(i18n.KeySaveAsNew)
	}
	return ""
}

// classifySaveAsName says what saving editing as typed would do. Names
// compare ignoring case and surrounding spaces, against every name an
// instrument goes by — its current name, its library name, and a shipped
// instrument's name in every language.
func classifySaveAsName(typed string, editing instrument.ID) saveAsVerdict {
	key := normalizeSaveAsName(typed)
	if key == "" {
		return saveAsVerdict{kind: saveAsEmpty}
	}
	own := saveAsIdentity(editing)
	for _, n := range append(instrumentNames(own), audio.InstrumentDisplayName(editing)) {
		if normalizeSaveAsName(n) == key {
			return saveAsVerdict{kind: saveAsOver, name: audio.InstrumentDisplayName(editing)}
		}
	}
	for _, id := range audio.LibraryIDs() {
		if id == own {
			continue
		}
		for _, n := range instrumentNames(id) {
			if normalizeSaveAsName(n) == key {
				return saveAsVerdict{kind: saveAsTaken, name: strings.TrimSpace(n)}
			}
		}
	}
	return saveAsVerdict{kind: saveAsNew}
}

// saveAsIdentity is the library instrument a Save As over editing saves to.
func saveAsIdentity(editing instrument.ID) instrument.ID {
	if doc, ok := audio.InstrumentDoc(editing); ok {
		return audio.InstrumentLibraryIdentity(editing, doc)
	}
	return editing
}

// instrumentNames lists every name id goes by.
func instrumentNames(id instrument.ID) []string {
	names := append(i18n.InstrumentNames(id), audio.InstrumentDisplayName(id))
	if d, ok := audio.UserLibraryDoc(id); ok {
		names = append(names, d.Name)
	}
	return names
}

func normalizeSaveAsName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
