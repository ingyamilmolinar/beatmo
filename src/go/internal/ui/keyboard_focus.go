package ui

// Keyboard input is not spatial, so a surface that takes typed or caret keys CLAIMS the keyboard
// while active; grid shortcuts run only when nothing claims it. This replaced a denylist that missed sources.

// Implementations MUST be nil-safe on the receiver.
type KeyboardClaimant interface {
	ClaimsKeyboard() bool
}

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

// While the picker is open its cursor takes the arrows, which must not also pan the camera. The
// sidebar is a Game-level surface, so Game.KeyboardClaimed unions it in.
func (sb *NodeSidebar) ClaimsKeyboard() bool {
	return sb != nil && sb.openPicker() != nil
}

// The single TOTAL ownership predicate for the whole window. Add new surfaces here (or to
// DrumView.KeyboardClaimed), never inline at a shortcut site; partial claims use KeyboardKeyClaimant.
func (g *Game) KeyboardClaimed() bool {
	if g == nil {
		return false
	}
	if g.sidebar.ClaimsKeyboard() {
		return true
	}
	return g.drum != nil && g.drum.KeyboardClaimed()
}

// The WHOLE stack, not just the top: a tooltip landing on top would otherwise drop the claim while
// the claiming overlay is still open.
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

// The WAV-naming box is checked through Focused(): it is not portal-hosted.
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
	// Portal claimants live in the global overlay subtree.
	if dv.overlayTree != nil && dv.overlayTree.OwnsKeyboard() {
		return true
	}
	// An armed control is deliberately absent: it owns only the arrows (a partial claimant), so Space
	// and [ ] still work.
	if dv.saveAsDialog != nil && dv.saveAsDialog.ClaimsKeyboard() {
		return true
	}
	if dv.nameBox != nil && dv.nameBox.Focused() {
		return true
	}
	return false
}
