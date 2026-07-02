package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// synthSaveAsDialog is the inline modal that prompts the user for a custom
// display name when cloning a synth recipe via Save As. The dialog lives
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
	instID     string
	baseID     string
	suggestion string
	// title overrides the dialog heading. Empty defaults to the synth-recipe
	// wording. The Sampler tab sets "Save sample as…".
	title string
	// confirm runs on OK with the trimmed typed name. When nil, the dialog
	// falls back to the synth-recipe Save As (back-compat for the synth tab).
	confirm func(name string)
}

const (
	synthSaveAsDialogWidth  = 360
	synthSaveAsDialogHeight = 140
	synthSaveAsDialogMaxLen = 48
)

// openSaveAsDialog initializes dv.saveAsDialog with a suggested default name
// derived from the active recipe's DisplayName. No-op when no instrument /
// recipe is bound. Idempotent — repeated calls re-suggest but don't lose any
// in-progress text.
func (dv *DrumView) openSaveAsDialog() {
	if dv == nil {
		return
	}
	instID := dv.synthTabActiveInstrument()
	if instID == "" {
		return
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return
	}
	reg := audio.RecipeRegistrations()[recipeID]
	suggestion := recipeID + " (saved)"
	if reg != nil && reg.DisplayName != "" {
		suggestion = reg.DisplayName + " (saved)"
	}
	// Idempotent: don't blow away an open dialog the user is mid-typing.
	if dv.saveAsDialog != nil {
		return
	}
	dlg := &synthSaveAsDialog{
		instID:     instID,
		baseID:     recipeID,
		suggestion: suggestion,
		title:      "Save preset as…",
		confirm:    func(name string) { dv.SaveActiveRecipeAs(name) },
	}
	dlg.layout(dv.eqPanelZone.ContentRect())
	dlg.textInput = NewTextInput(dlg.inputRect(), BPMBoxStyle)
	dlg.textInput.MaxLen = synthSaveAsDialogMaxLen
	dlg.textInput.InputMode = "text"
	dlg.textInput.SetText(suggestion)
	dlg.textInput.SetFocus(true)
	dlg.okBtn = NewButtonKey(i18n.KeyOK, ComponentButtonPrimary, nil)
	dlg.cancelBtn = NewButtonKey(i18n.KeyCancel, ComponentButtonSecondary, nil)
	dlg.okBtn.SetRect(dlg.okBtnRect())
	dlg.cancelBtn.SetRect(dlg.cancelBtnRect())
	dv.saveAsDialog = dlg
}

// ConfirmSaveAsDialog finalises the open dialog: snapshots the typed name,
// calls SaveActiveRecipeAs(displayName), closes the dialog. Public so the
// HitArea handler + tests + JS bridge can drive it uniformly.
func (dv *DrumView) ConfirmSaveAsDialog() {
	if dv == nil || dv.saveAsDialog == nil {
		return
	}
	name := strings.TrimSpace(dv.saveAsDialog.textInput.Value())
	confirm := dv.saveAsDialog.confirm
	dv.saveAsDialog = nil
	if confirm != nil {
		confirm(name)
		return
	}
	dv.SaveActiveRecipeAs(name)
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
	dlg.textInput.Update()
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
	title := dlg.title
	if title == "" {
		title = "Save preset as…"
	}
	drawRoundedRect(dst, dlg.rect, TokenSurface2(), RadiusMD, true)
	drawRoundedRect(dst, dlg.rect, TokenBorderMedium(), RadiusMD, false)
	DrawTextStyled(dst, title,
		dlg.rect.Min.X+SpaceMD, dlg.rect.Min.Y+SpaceMD, RolePanelTitle, TokenTextPrimary())
	dlg.textInput.Draw(dst)
	dlg.okBtn.Draw(dst)
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

// openSamplerSaveAsDialog opens the Save As name prompt for the Sampler tab.
// Reuses the shared dialog widget; confirm routes to samplerSaveAsConfirmed.
// No-op without a working buffer or when a dialog is already open.
func (dv *DrumView) openSamplerSaveAsDialog() {
	if dv == nil || dv.saveAsDialog != nil {
		return
	}
	if !dv.sampler.hasBuffer() {
		return
	}
	base := dv.sampler.captureID
	suggestion := samplerSaveAsSuggestion(dv, base)
	dlg := &synthSaveAsDialog{
		instID:     base,
		baseID:     base,
		suggestion: suggestion,
		title:      "Save sample as…",
		confirm:    func(name string) { dv.samplerSaveAsConfirmed(name) },
	}
	dlg.layout(dv.eqPanelZone.ContentRect())
	dlg.textInput = NewTextInput(dlg.inputRect(), BPMBoxStyle)
	dlg.textInput.MaxLen = synthSaveAsDialogMaxLen
	dlg.textInput.InputMode = "text"
	dlg.textInput.SetText(suggestion)
	dlg.textInput.SetFocus(true)
	dlg.okBtn = NewButtonKey(i18n.KeyOK, ComponentButtonPrimary, nil)
	dlg.cancelBtn = NewButtonKey(i18n.KeyCancel, ComponentButtonSecondary, nil)
	dlg.okBtn.SetRect(dlg.okBtnRect())
	dlg.cancelBtn.SetRect(dlg.cancelBtnRect())
	dv.saveAsDialog = dlg
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

func (d *synthSaveAsDialog) okBtnRect() image.Rectangle {
	inset := SpaceMD
	btnW, btnH := 80, 32
	return image.Rect(
		d.rect.Max.X-inset-btnW,
		d.rect.Max.Y-inset-btnH,
		d.rect.Max.X-inset,
		d.rect.Max.Y-inset,
	)
}

func (d *synthSaveAsDialog) cancelBtnRect() image.Rectangle {
	inset := SpaceMD
	btnW, btnH := 80, 32
	return image.Rect(
		d.rect.Max.X-inset-btnW-SpaceSM-btnW,
		d.rect.Max.Y-inset-btnH,
		d.rect.Max.X-inset-btnW-SpaceSM,
		d.rect.Max.Y-inset,
	)
}
