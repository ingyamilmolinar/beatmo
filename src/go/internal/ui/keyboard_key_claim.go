package ui

import "github.com/hajimehoshi/ebiten/v2"

// The PARTIAL half of keyboard ownership: a value control over a tab wants the arrows and nothing
// else (Space must still play). It declares the keys it consumes this frame and handles them; claim
// and handle are ONE interface so they cannot drift apart.

// MUST be nil-safe on the receiver and report nil once closed.
type KeyboardKeyClaimant interface {
	// nil when it owns none. Arrows in BOTH spellings (KeyArrowUp and KeyUp): real Ebiten aliases
	// them, the test stub does not.
	ClaimedKeys() []ebiten.Key
	HandleClaimedKeys() bool
}

// Both spellings, for the stub-alias reason above.
var (
	keysStepUp    = []ebiten.Key{ebiten.KeyArrowUp, ebiten.KeyUp}
	keysStepDown  = []ebiten.Key{ebiten.KeyArrowDown, ebiten.KeyDown}
	keysStepLeft  = []ebiten.Key{ebiten.KeyArrowLeft, ebiten.KeyLeft}
	keysStepRight = []ebiten.Key{ebiten.KeyArrowRight, ebiten.KeyRight}
)

// Precomputed because ClaimedKeys runs every frame; read-only.
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

// Up/Down step the value; Left/Right change the step resolution.
func valueAndResolutionStepKeys() []ebiten.Key { return keysValueAndResolutionStep }

// Folds the alias spellings into one test.
func anyKeyJustPressed(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if isKeyJustPressed(k) {
			return true
		}
	}
	return false
}

// TOP first; the whole stack is scanned so a tooltip on top cannot hand the arrows back to the camera.
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

// Add new per-key surfaces here, never as an inline isKeyPressed check at the control.
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

// The topmost claimant acts; the whole stack blocks. Two controls stepping on one press is worse
// than one going inert, and a lower visible claimant must never hand the arrows back. nil = nothing claims.
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
