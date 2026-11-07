//go:build test

package ui

import (
	"image"
	"testing"
)

// Mobile only: stopped, play wears the saturated PrimaryActionStyle so it dominates; playing, it recedes to the
// profile's PlayBtnStyle so the active state does not shout.
func TestMobilePlayPrimaryActionStyleSwap(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 390, 96))

	// Stopped state — play button should be PrimaryActionStyle.
	z.SetPlaying(false)
	if z.playBtn.Style != PrimaryActionStyle {
		t.Errorf("stopped state: play button style %T should be PrimaryActionStyle (colAccent fill)", z.playBtn.Style)
	}
	if got := PrimaryActionStyle.Fill; got != colAccent {
		t.Errorf("PrimaryActionStyle.Fill = %v, want colAccent %v", got, colAccent)
	}

	// Playing state — play button should fall back to the profile recede style.
	z.SetPlaying(true)
	prof := Profile()
	if z.playBtn.Style != prof.PlayBtnStyle {
		t.Errorf("playing state: play button style should revert to Profile.PlayBtnStyle, got %T", z.playBtn.Style)
	}
}

func TestDesktopPlayKeepsDefaultStyle(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 1280, 96))

	defaultStyle := Profile().PlayBtnStyle
	z.SetPlaying(false)
	if z.playBtn.Style != defaultStyle {
		t.Errorf("desktop stopped state: play button must keep default profile style, got %T", z.playBtn.Style)
	}
	z.SetPlaying(true)
	if z.playBtn.Style != defaultStyle {
		t.Errorf("desktop playing state: play button must keep default profile style, got %T", z.playBtn.Style)
	}
}
