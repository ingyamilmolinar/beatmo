package ui

// keyboard_focus.go — the keyboard-ownership contract.
//
// Keyboard input is not spatial, so "who gets the keys?" cannot be answered by
// the HitIndex. Instead, any surface that accepts typed/caret keys *claims* the
// keyboard while it is active. Grid-level keyboard shortcuts (arrow-pan, zoom,
// Space, BPM±, tab-select, "?") run only when nothing claims the keyboard —
// see handleGlobalShortcuts. This replaces the previous denylist that had to
// enumerate every text-input source (and missed several).

// KeyboardClaimant is implemented by anything that owns the keyboard while it is
// open/focused: portal-hosted modal components (rename, instrument menu) and the
// inline Save As dialog. Implementations MUST be nil-safe on the receiver.
type KeyboardClaimant interface {
	ClaimsKeyboard() bool
}

// OwnsKeyboard reports whether this tree (a focused zone, or a portal overlay
// that claims the keyboard) currently owns keyboard input.
func (t *DrumViewTree) OwnsKeyboard() bool {
	if t == nil {
		return false
	}
	if t.focusedZone != "" {
		return true
	}
	if t.portal != nil && t.portal.ClaimsKeyboard() {
		return true
	}
	return false
}

// ClaimsKeyboard reports whether this portal's top overlay claims the keyboard.
func (p *OverlayPortal) ClaimsKeyboard() bool {
	if p == nil {
		return false
	}
	if c, ok := p.TopOverlay().(KeyboardClaimant); ok {
		return c.ClaimsKeyboard()
	}
	return false
}

// KeyboardClaimed reports whether ANY bottom-panel surface currently owns the
// keyboard: either subtree (focused zone or claiming portal overlay), the shared
// inline numeric editor (transport BPM / EQ dB / synth knob), the Save As
// dialog, or the WAV-naming box. The grid keyboard handler yields when true.
// Note: the nameBox (WAV-naming) is checked directly via .Focused() and is NOT
// a KeyboardClaimant — it is not portal-hosted and carries no ClaimsKeyboard method.
func (dv *DrumView) KeyboardClaimed() bool {
	if dv == nil {
		return false
	}
	if dv.tree != nil && dv.tree.OwnsKeyboard() {
		return true
	}
	if dv.audioTree != nil && dv.audioTree.OwnsKeyboard() {
		return true
	}
	// Portal-hosted keyboard claimants (rename field, instrument-menu search)
	// now live in the single global overlay subtree, so consult it too.
	if dv.overlayTree != nil && dv.overlayTree.OwnsKeyboard() {
		return true
	}
	if dv.valueEditorActive() {
		return true
	}
	if dv.saveAsDialog != nil && dv.saveAsDialog.ClaimsKeyboard() {
		return true
	}
	if dv.nameBox != nil && dv.nameBox.Focused() {
		return true
	}
	return false
}
