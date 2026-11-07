package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The confirm button says what will happen as the user types ("Save over <name>" / "Save as new"), and
// another instrument's name is refused in place. Its own HitArea blocks clicks behind it.
type synthSaveAsDialog struct {
	rect       image.Rectangle
	textInput  *TextInput
	okBtn      *Button
	cancelBtn  *Button
	instID     instrument.ID
	suggestion string
	// Classified once per change.
	typed   string
	verdict saveAsVerdict
}

const (
	synthSaveAsDialogWidth  = 360
	synthSaveAsDialogHeight = 140
	synthSaveAsDialogMaxLen = 48
)

// The Synth and Sampler share the dialog and are never open at once.
const saveAsMobileInputID = "save-as-name"

// openSaveAsDialog opens Save As for the Synth tab's instrument.
func (dv *DrumView) openSaveAsDialog() { dv.openSaveAsDialogFor(dv.synthTabActiveInstrument()) }

// Idempotent, so a repeated call never loses in-progress text.
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

// A refused or empty name keeps the dialog open.
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

// The seam the universal Esc handler uses, so Esc cancels the dialog instead of falling through to Stop.
// Portal-hosted inputs are handled by the portal's Esc path.
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
	// Mirror the native <input> so OK confirms what the user sees.
	if v, ok := dlg.textInput.NativeLiveValue(); ok {
		dlg.textInput.SetText(v)
	}
	dlg.textInput.Update()
	dlg.classify()
	if isKeyPressed(ebiten.KeyEnter) {
		dv.ConfirmSaveAsDialog()
		return
	}
	// Esc is owned by the universal handler, the single Esc authority; never handle it here.
}

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

func (d *synthSaveAsDialog) Focused() bool {
	if d == nil || d.textInput == nil {
		return false
	}
	return d.textInput.Focused()
}

// While its input is focused, every key belongs to it.
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

// "" when the name can be saved.
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

// Injects text without the keyboard plumbing, for tests and the JS bridge.
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

// So a browser test can click the visible centres without hit-area tags.
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

// OK/Cancel sit above the frame, which consumes stray taps so a click inside the dialog never dismisses it.
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

// "Save over <name>" is long; no wider than the room beside Cancel.
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

// Case- and space-insensitive, against every name an instrument goes by, a shipped one's in every language.
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
