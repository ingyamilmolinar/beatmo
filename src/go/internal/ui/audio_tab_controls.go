package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// tabControls is the control header owned by a single audio-panel tab. Each
// tab's controls are an independent component — no tab shares a button widget
// or visibility state with another. The dispatcher (EQPanelZone) asks the
// active tab's component for its header height, lays it into the strip just
// below the shared tab-switcher row, draws it, and routes its hit areas. This
// is the per-tab ownership model that ChainPanelZone and the Synth header
// already follow.
type tabControls interface {
	// HeaderH returns the control-header height in pixels (0 = no header).
	HeaderH() int
	// Layout positions the buttons inside header (the strip reserved at the
	// top of the tab's content region, directly below the tab-switcher row).
	Layout(header image.Rectangle)
	// Draw renders the buttons.
	Draw(dst *ebiten.Image)
	// HitAreas returns the buttons' hit areas (z-indexed above the panel body).
	HitAreas() []HitArea
	// SyncFreeze updates the freeze button's glyph/color from the analyzer's
	// global frozen state. No-op for components without a freeze button.
	SyncFreeze(frozen bool)
}

// audioControlHeaderH is the desktop height of a per-tab control header — the
// row of tab-specific pills directly below the shared tab-switcher row. Mirrors
// stickyBarH so the two rows read as a stacked pair.
const audioControlHeaderH = 26

// controlHeaderHeight returns the per-tab control-header height (taller on
// mobile so the freeze pill meets the touch-min, mirroring stickyBarHeight).
func controlHeaderHeight() int {
	if Profile().IsMobile() {
		return 36
	}
	return audioControlHeaderH
}

// newFreezePill builds a freeze/resume toggle pill wired to onFreeze (which
// returns the new frozen state). Each tab owns its own instance — there is no
// shared freeze widget. The "||" (capturing) / ">" (frozen) glyphs are a
// DESIGN.md §5d permitted text-glyph exception.
func newFreezePill(onFreeze func() bool) *Button {
	b := NewButton("||", InstButtonStyle, nil)
	b.TextColor = colTextSecondary
	b.OnClick = func() {
		if onFreeze == nil {
			return
		}
		applyFreezeVisual(b, onFreeze())
	}
	return b
}

// applyFreezeVisual sets the freeze pill's glyph + color from a frozen bool.
func applyFreezeVisual(b *Button, frozen bool) {
	if b == nil {
		return
	}
	if frozen {
		b.Text = ">"
		b.TextColor = colAccent
	} else {
		b.Text = "||"
		b.TextColor = colTextSecondary
	}
}
