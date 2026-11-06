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

// ClaimsKeyboard reports whether the node sidebar owns the keyboard.
//
// It does while the note-matrix picker is open: the picker's cursor moves on
// the arrow keys, and the grid's arrow-pan must not run on the same press —
// one keypress would otherwise both move the note cursor and pan the camera.
//
// The sidebar is a Game-level surface (it renders through GridTree, not the
// DrumView trees), which is why Game.KeyboardClaimed exists to union it with
// DrumView.KeyboardClaimed rather than it being reachable from there.
func (sb *NodeSidebar) ClaimsKeyboard() bool {
	return sb != nil && sb.openPicker() != nil
}

// KeyboardClaimed is the single keyboard-ownership predicate for the WHOLE
// window: the bottom panel's surfaces plus the Game-level ones (currently the
// node sidebar). handleGlobalShortcuts consults this, so every gated grid
// shortcut yields to whichever surface is active.
//
// Add new keyboard-owning surfaces HERE (or to DrumView.KeyboardClaimed for
// bottom-panel ones) — never as an inline check at a shortcut site.
//
// This is the TOTAL claim ("every key is mine", the right answer for a focused
// text field). A control that owns only a few keys — a value popup wanting the
// arrows — declares them via KeyboardKeyClaimant instead; see
// keyboard_key_claim.go.
func (g *Game) KeyboardClaimed() bool {
	if g == nil {
		return false
	}
	if g.sidebar.ClaimsKeyboard() {
		return true
	}
	return g.drum != nil && g.drum.KeyboardClaimed()
}

// ClaimsKeyboard reports whether ANY overlay in this portal claims the keyboard.
//
// The WHOLE stack is scanned, not just the top. Consulting only TopOverlay made
// the claim depend on nothing else having opened since: a tooltip, a
// notification or any transient surface landing on top silently dropped it, and
// the grid took the arrows back while the claiming overlay was still open and
// visible on screen. That is not a state the claiming component can detect or
// defend against, so the portal has to answer for its whole stack.
//
// Only one overlay normally claims at a time, so scanning is safe: the answer
// is "is a keyboard-owning surface open", which is exactly what the grid needs
// in order to yield.
func (p *OverlayPortal) ClaimsKeyboard() bool {
	if p == nil {
		return false
	}
	for _, e := range p.stack {
		if c, ok := e.Overlay.(KeyboardClaimant); ok && c.ClaimsKeyboard() {
			return true
		}
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
	// NOTE: an armed Synth/Sampler control is deliberately NOT here. It owns
	// only the arrows, not the whole keyboard, so it declares itself as a
	// PARTIAL claimant instead (armedControlKeyClaimant, keyboard_key_claim.go)
	// — Space still plays and [ ] still zoom while a control is armed.
	if dv.saveAsDialog != nil && dv.saveAsDialog.ClaimsKeyboard() {
		return true
	}
	if dv.nameBox != nil && dv.nameBox.Focused() {
		return true
	}
	return false
}
