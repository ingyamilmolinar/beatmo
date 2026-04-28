//go:build test

package ui

import (
	"image/color"
	"testing"
)

// TestSyncToggleVisual covers the package-level toggle-visual helper used by
// the track button and the row-rack toggles. The helper is a pure setter and
// supports two patterns:
//
//   - play/stop pattern: same on/off style; icon glyph + icon color flip
//     (caller passes both icons and both icon colors).
//   - row-control pattern: different on/off styles; icon and icon color
//     untouched (caller passes "" / "" and nil / nil).
func TestSyncToggleVisual(t *testing.T) {
	t.Run("nil button is a no-op", func(t *testing.T) {
		// Must not panic.
		syncToggleVisual(nil, true, InstButtonStyle, InstButtonStyle, IconTrack, IconTrackOff, color.White, color.Black)
	})

	t.Run("on=true sets onStyle, onIcon, onIconColor, pressed=true", func(t *testing.T) {
		btn := &Button{}
		btn.Style = InstButtonStyle
		syncToggleVisual(btn, true,
			MuteActiveStyle, InstButtonStyle,
			IconTrack, IconTrackOff,
			color.White, color.Black,
		)
		if btn.Icon != string(IconTrack) {
			t.Errorf("Icon = %q, want %q", btn.Icon, IconTrack)
		}
		if got, want := btn.IconColor, color.Color(color.White); got != want {
			t.Errorf("IconColor = %v, want %v", got, want)
		}
		if !btn.pressed {
			t.Errorf("pressed = false, want true")
		}
	})

	t.Run("on=false sets offStyle, offIcon, offIconColor, pressed=false", func(t *testing.T) {
		btn := &Button{}
		btn.pressed = true // start pressed; helper should clear it
		syncToggleVisual(btn, false,
			MuteActiveStyle, InstButtonStyle,
			IconTrack, IconTrackOff,
			color.White, color.Black,
		)
		if btn.Icon != string(IconTrackOff) {
			t.Errorf("Icon = %q, want %q", btn.Icon, IconTrackOff)
		}
		if got, want := btn.IconColor, color.Color(color.Black); got != want {
			t.Errorf("IconColor = %v, want %v", got, want)
		}
		if btn.pressed {
			t.Errorf("pressed = true, want false")
		}
	})

	t.Run("play/stop pattern: same style on both flips icon + tint only", func(t *testing.T) {
		btn := &Button{}
		btn.Icon = "previously-set"
		// Both onStyle == offStyle: chrome must not change between states; only
		// icon glyph and icon color flip with state. Mirrors how SetPlaying
		// treats the play button.
		syncToggleVisual(btn, true,
			TransportMiscStyle, TransportMiscStyle,
			IconTrack, IconTrackOff,
			color.White, color.Black,
		)
		if btn.Icon != string(IconTrack) {
			t.Errorf("on=true Icon = %q, want %q", btn.Icon, IconTrack)
		}
		syncToggleVisual(btn, false,
			TransportMiscStyle, TransportMiscStyle,
			IconTrack, IconTrackOff,
			color.White, color.Black,
		)
		if btn.Icon != string(IconTrackOff) {
			t.Errorf("on=false Icon = %q, want %q", btn.Icon, IconTrackOff)
		}
	})

	t.Run("empty IconID leaves Icon untouched (row-control pattern)", func(t *testing.T) {
		btn := &Button{}
		btn.Icon = "preserved"
		syncToggleVisual(btn, true,
			MuteActiveStyle, InstButtonStyle,
			"", "", // empty -> don't touch
			color.White, color.Black,
		)
		if btn.Icon != "preserved" {
			t.Errorf("Icon = %q, want preserved (helper should not touch when both icons are empty)", btn.Icon)
		}
	})

	t.Run("nil iconColor leaves IconColor untouched", func(t *testing.T) {
		btn := &Button{}
		btn.IconColor = color.RGBA{R: 1, G: 2, B: 3, A: 4}
		syncToggleVisual(btn, true,
			MuteActiveStyle, InstButtonStyle,
			"", "",
			nil, nil, // nil -> don't touch
		)
		if got := btn.IconColor; got != (color.RGBA{R: 1, G: 2, B: 3, A: 4}) {
			t.Errorf("IconColor changed unexpectedly: got %v", got)
		}
	})

	t.Run("only one icon set is treated as both empty (row-control pattern)", func(t *testing.T) {
		btn := &Button{}
		btn.Icon = "preserved"
		// Mixed: only onIcon provided. Helper should NOT update Icon — it
		// only switches when both icons are non-empty (otherwise the off
		// state would have an undefined glyph).
		syncToggleVisual(btn, true,
			MuteActiveStyle, InstButtonStyle,
			IconTrack, "",
			nil, nil,
		)
		if btn.Icon != "preserved" {
			t.Errorf("Icon = %q, want preserved (one-sided icon spec must be ignored)", btn.Icon)
		}
	})
}
