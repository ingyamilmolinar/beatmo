//go:build test

package ui

import (
	"testing"
)

func TestTokenDestructiveColors(t *testing.T) {
	if TokenDeleteFill() == (TokenDeleteFill()) { // ensure it compiles
	}
	del := TokenDeleteFill()
	if del.R == 0 && del.G == 0 && del.B == 0 {
		t.Fatal("TokenDeleteFill must not be zero/black")
	}
	if TokenMuteActiveFill() == (TokenMuteActiveFill()) {
	}
	mute := TokenMuteActiveFill()
	if mute.R == 0 && mute.G == 0 && mute.B == 0 {
		t.Fatal("TokenMuteActiveFill must not be zero/black")
	}
	// The three reds must be distinct roles (different values).
	stop := TokenStopRed()
	if stop == mute || mute == del || stop == del {
		t.Fatal("stop/mute/delete reds must be three distinct color values")
	}
}

func TestTokenSurfaces(t *testing.T) {
	s1, s2, s3 := TokenSurface1(), TokenSurface2(), TokenSurface3()
	// Each level should be brighter (higher R) than the one below.
	if s1.R >= s2.R || s2.R >= s3.R {
		t.Errorf("surface levels must increase in brightness: s1=%v s2=%v s3=%v", s1, s2, s3)
	}
}

func TestTokenAccentConsistency(t *testing.T) {
	acc := TokenAccent()
	// Single chrome accent — sunset-gold #FFB30A (the cyan→gold repaint;
	// DESIGN.md primary / on-surface-accent both resolve to #FFB30A).
	if acc.R != 255 || acc.G != 179 || acc.B != 10 {
		t.Errorf("TokenAccent must be #FFB30A, got R=%d G=%d B=%d", acc.R, acc.G, acc.B)
	}
}

func TestTokenTextHierarchy(t *testing.T) {
	pri := TokenTextPrimary()
	sec := TokenTextSecondary()
	dis := TokenTextDisabled()
	// Primary should be brightest, disabled dimmest.
	if pri.R <= sec.R || sec.R <= dis.R {
		t.Errorf("text hierarchy brightness wrong: primary=%v secondary=%v disabled=%v", pri, sec, dis)
	}
}
