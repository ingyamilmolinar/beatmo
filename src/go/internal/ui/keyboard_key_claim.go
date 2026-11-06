package ui

import "github.com/hajimehoshi/ebiten/v2"

// keyboard_key_claim.go — the PARTIAL (per-key) half of the keyboard-ownership
// contract.
//
// keyboard_focus.go answers "does a surface own the WHOLE keyboard?" — the right
// question for a focused text field, where every keystroke is typed text. A
// value control that pops up over a tab (the Sampler/EQ precision wheel, a
// volume slider, an armed Synth control) owns something much narrower: it wants
// the arrows and nothing else. Space must still play, [ ] must still zoom.
//
// A partial claimant therefore declares the keys it consumes THIS FRAME and
// handles them; the router suppresses exactly those keys for the gated grid /
// transport / audio-panel shortcut nodes and lets everything else through
// untouched.
//
// Claiming and handling are ONE interface on purpose. The two halves drifting
// apart is the bug this file exists to prevent: the Sampler wheel popup handled
// no arrow keys AND claimed none, so opening the Pitch control and pressing Up
// panned the camera instead of moving the value.

// KeyboardKeyClaimant is implemented by a surface that owns only SPECIFIC keys
// while it is open. Implementations MUST be nil-safe on the receiver and MUST
// report nil keys once closed.
type KeyboardKeyClaimant interface {
	// ClaimedKeys returns the keys this surface consumes for the current frame,
	// or nil when it owns none. Arrow keys must be declared in BOTH spellings
	// (KeyArrowUp *and* KeyUp): real Ebiten aliases them, but the -tags test
	// ebitestub declares them as distinct constants.
	ClaimedKeys() []ebiten.Key
	// HandleClaimedKeys acts on those keys for the current frame and reports
	// whether anything changed.
	HandleClaimedKeys() bool
}

// Shared arrow-key spellings for partial claimants. Both spellings are listed
// for the ebitestub-alias reason above (see menu_keynav.go's onEdgeAny, which
// hit the same hazard).
var (
	keysStepUp    = []ebiten.Key{ebiten.KeyArrowUp, ebiten.KeyUp}
	keysStepDown  = []ebiten.Key{ebiten.KeyArrowDown, ebiten.KeyDown}
	keysStepLeft  = []ebiten.Key{ebiten.KeyArrowLeft, ebiten.KeyLeft}
	keysStepRight = []ebiten.Key{ebiten.KeyArrowRight, ebiten.KeyRight}
)

// The two standing claim sets, precomputed: ClaimedKeys runs every frame a
// control is open, so it returns these shared slices rather than building one.
// Callers MUST treat them as read-only.
var (
	keysValueStep              = concatKeys(keysStepUp, keysStepDown)
	keysValueAndResolutionStep = concatKeys(keysStepUp, keysStepDown, keysStepLeft, keysStepRight)
)

func concatKeys(sets ...[]ebiten.Key) []ebiten.Key {
	var out []ebiten.Key
	for _, s := range sets {
		out = append(out, s...)
	}
	return out
}

// valueStepKeys is the claim of a control whose value steps on Up/Down only.
func valueStepKeys() []ebiten.Key { return keysValueStep }

// valueAndResolutionStepKeys is the claim of a control whose value steps on
// Up/Down and whose step RESOLUTION changes on Left/Right.
func valueAndResolutionStepKeys() []ebiten.Key { return keysValueAndResolutionStep }

// anyKeyJustPressed reports whether ANY of keys had a press edge this frame.
// Used to fold the arrow-key alias spellings into one test.
func anyKeyJustPressed(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if isKeyJustPressed(k) {
			return true
		}
	}
	return false
}

// KeyClaimants appends every partial claimant in this portal's stack, TOP first.
// The whole stack is scanned for the same reason ClaimsKeyboard scans it: a
// tooltip or notification landing on top must not silently hand the arrows back
// to the camera while the claiming control is still open and visible.
func (p *OverlayPortal) KeyClaimants(out []KeyboardKeyClaimant) []KeyboardKeyClaimant {
	if p == nil {
		return out
	}
	for i := len(p.stack) - 1; i >= 0; i-- {
		if c, ok := p.stack[i].Overlay.(KeyboardKeyClaimant); ok {
			out = append(out, c)
		}
	}
	return out
}

// KeyClaimants appends this tree's portal-hosted partial claimants.
func (t *DrumViewTree) KeyClaimants(out []KeyboardKeyClaimant) []KeyboardKeyClaimant {
	if t == nil {
		return out
	}
	return t.portal.KeyClaimants(out)
}

// KeyClaimants appends every partial claimant reachable from the bottom panel:
// the three overlay subtrees plus the inline armed Synth/Sampler control (which
// is not portal-hosted).
//
// Add new per-key surfaces HERE (or to Game.KeyClaimants for grid-pane ones) —
// never as an inline isKeyPressed check at the control's own update site.
func (dv *DrumView) KeyClaimants(out []KeyboardKeyClaimant) []KeyboardKeyClaimant {
	if dv == nil {
		return out
	}
	out = dv.tree.KeyClaimants(out)
	out = dv.audioTree.KeyClaimants(out)
	out = dv.overlayTree.KeyClaimants(out)
	if c := dv.armedControlKeyClaimant(); c != nil {
		out = append(out, c)
	}
	return out
}

// KeyClaimants returns every open partial claimant for the WHOLE window.
func (g *Game) KeyClaimants() []KeyboardKeyClaimant {
	if g == nil {
		return nil
	}
	return g.drum.KeyClaimants(nil)
}

// dispatchClaimedKeys lets the TOPMOST open partial claimant act on the keys it
// owns and returns the UNION of every open claimant's keys, so the gated
// shortcut router can suppress them. Returns nil when nothing claims, which is
// the zero-cost common case.
//
// Top-most ACTS, whole stack BLOCKS — the same split ClaimsKeyboard uses. Only
// one value control is open at a time in practice (CloseAllPopups, one armed
// control per tab), but if that ever stopped holding, two controls both stepping
// on one arrow press is a much worse failure than one of them going inert, and
// a lower claimant whose control is still on screen must never hand the arrows
// back to the camera.
func (g *Game) dispatchClaimedKeys() map[ebiten.Key]bool {
	claimants := g.KeyClaimants()
	if len(claimants) == 0 {
		return nil
	}
	var blocked map[ebiten.Key]bool
	handled := false
	for _, c := range claimants {
		keys := c.ClaimedKeys()
		if len(keys) == 0 {
			continue
		}
		if blocked == nil {
			blocked = make(map[ebiten.Key]bool, len(keys))
		}
		for _, k := range keys {
			blocked[k] = true
		}
		if !handled {
			c.HandleClaimedKeys()
			handled = true
		}
	}
	return blocked
}
