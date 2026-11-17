//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// A theme applied AFTER widgets were built (the normal case: a circuit theme
// arrives on import, an editor pick happens mid-session) must still reach
// styles and colors that were COPIED into widgets at construction.
func TestCapturedStylesAndColorsFollowLaterTheme(t *testing.T) {
	// Captured before the theme, exactly like transport_zone.go / eq_panel_zone.go do.
	btnStyle := InstButtonStyle
	inputStyle := BPMBoxStyle
	b := NewButton("x", InstButtonStyle, nil)
	b.IconColor = colTextSecondary
	b.TextColor = colTextSecondary
	in := &TextInput{Style: BPMBoxStyle, TextColor: colTextSecondary}
	_ = in

	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"surfaces": "mint-500", "text": "tangerine-100"}})
	defer restore()
	liveFill := Spec(ComponentButtonRowControl).Fill
	if liveFill == genColorDefaultByName["surface-2"] {
		t.Fatal("precondition: surfaces theme did not change surface-2")
	}
	r := image.Rect(0, 0, 40, 24)
	dst := ebiten.NewImage(48, 32)
	fills := collectAllFills(t, func() { btnStyle.Draw(dst, r, false, false) })
	if len(fills) == 0 || fills[0].Color != liveFill {
		t.Fatalf("copied ButtonStyle drew %+v, want live fill %v", fills, liveFill)
	}
	fills = collectAllFills(t, func() { inputStyle.Draw(dst, r, false, false) })
	if len(fills) == 0 || fills[0].Color != Spec(ComponentInputField).Fill {
		t.Fatalf("copied TextInputStyle drew %+v, want live %v", fills, Spec(ComponentInputField).Fill)
	}
	if got := b.effectiveIconColor(); got != colTextSecondary || got == genColorDefaultByName["on-surface-muted"] {
		t.Fatalf("captured icon color = %v, want live on-surface-muted %v", got, colTextSecondary)
	}
	if got := b.effectiveTextColor(); got != colTextSecondary {
		t.Fatalf("captured text color = %v, want live %v", got, colTextSecondary)
	}
	// A non-token color (instrument tint) passes through untouched.
	custom := color.RGBA{1, 2, 3, 255}
	if liveCaptured(custom) != custom {
		t.Fatal("non-token color must pass through liveCaptured")
	}
	// A shared hex resolves to the CHROME token: default primary == viz-bar.
	restore2 := SetThemeForTest(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "viz": "mint-300"}})
	defer restore2()
	if got := liveCaptured(genColorDefaultByName["primary"]); got != colAccent {
		t.Fatalf("captured accent resolved to %v, want live primary %v (not viz-bar %v)", got, colAccent, genColorVizBar)
	}
}

// The layout profile caches token colors at build time; a theme change must
// rebuild it so the transport re-copies live icon tints.
func TestProfileRebuiltOnThemeChange(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	before := Profile().StopIconColor
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"text": "mint-100"}})
	after := Profile().StopIconColor
	if before == after {
		t.Fatalf("profile StopIconColor not rebuilt after theme: %v", after)
	}
	if after != color.Color(colTextSecondary) {
		t.Fatalf("profile StopIconColor = %v, want live on-surface-muted %v", after, colTextSecondary)
	}
}
