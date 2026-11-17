//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// styleOf type-asserts a button's visual to the row-toggle visual so tests can
// read the resolved fill. Fails the test if the style isn't one (the per-row
// toggles and the kebab all resolve to it).
func styleOf(t *testing.T, b *Button) ButtonStyle {
	t.Helper()
	st, ok := b.Style.(rowToggleVisual)
	if !ok {
		t.Fatalf("button style is %T, want rowToggleVisual", b.Style)
	}
	return st.ButtonStyle
}

// TestRowMuteSoloFXShadesFollowInstrumentColor verifies that the per-row
// mute / solo / FX controls are rendered as shades of each row's *instrument
// color* (no longer a fixed magenta/cyan): mute darker than the hue, solo
// brighter, and two differently-colored rows get distinct fills.
func TestRowMuteSoloFXShadesFollowInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false })

	rows := makeTestRows(2)
	rows[0].Color = color.RGBA{190, 120, 60, 255} // orange
	rows[1].Color = color.RGBA{60, 150, 170, 255} // teal
	rows[0].Muted = true
	rows[1].Muted = true
	rows[0].Solo = true

	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 1000, 600))
	tree.Update()
	z.MarkDirty()
	screen := ebiten.NewImage(1000, 600)
	z.Draw(screen)

	if z.entries[0].muteBtn.Rect().Empty() || z.entries[1].muteBtn.Rect().Empty() {
		t.Fatal("mute buttons must be laid out for this test")
	}

	mute0 := styleOf(t, z.entries[0].muteBtn)
	mute1 := styleOf(t, z.entries[1].muteBtn)

	// Two different instrument colors → two different mute fills (no drift).
	if colorsEqual(mute0.Fill, mute1.Fill) {
		t.Errorf("rows with different instrument colors must have different mute fills: %v vs %v",
			mute0.Fill, mute1.Fill)
	}

	// Mute active fill is a darker shade of the instrument color.
	if lum(rowShadeToRGBA(mute0.Fill)) >= lum(rows[0].Color.(color.RGBA)) {
		t.Errorf("row0 mute fill should be darker than its instrument color: fill=%v color=%v",
			mute0.Fill, rows[0].Color)
	}

	// Solo active fill is a brighter shade of the instrument color.
	solo0 := styleOf(t, z.entries[0].soloBtn)
	if lum(rowShadeToRGBA(solo0.Fill)) <= lum(rows[0].Color.(color.RGBA)) {
		t.Errorf("row0 solo fill should be brighter than its instrument color: fill=%v color=%v",
			solo0.Fill, rows[0].Color)
	}

	// Inactive solo (row 1) is the NEUTRAL row-control body — identity at rest
	// lives in the label swatch, not in idle chips — with a muted letter.
	solo1 := styleOf(t, z.entries[1].soloBtn)
	if colorsEqual(solo0.Fill, solo1.Fill) {
		t.Error("active and inactive solo fills must differ")
	}
	if !colorsEqual(solo1.Fill, InstButtonStyle.fill()) {
		t.Errorf("inactive solo fill = %v, want the neutral row-control body %v", solo1.Fill, InstButtonStyle.fill())
	}
	if !colorsEqual(z.entries[1].soloBtn.TextColor, colTextSecondary) {
		t.Errorf("inactive solo letter = %v, want on-surface-muted", z.entries[1].soloBtn.TextColor)
	}
	// Every ACTIVE letter clears the WCAG 4.5:1 text contrast on its fill.
	for _, b := range []*Button{z.entries[0].muteBtn, z.entries[0].soloBtn, z.entries[1].muteBtn} {
		fill := styleOf(t, b).Fill
		if r := wcagContrast(relLuminance(fill), relLuminance(b.TextColor)); r < 4.5 {
			t.Errorf("%q letter %v on fill %v has contrast %.2f < 4.5", b.Text, b.TextColor, fill, r)
		}
	}
}

// TestRowKebabIsNeutral verifies the per-row ellipsis (kebab) chip wears the
// neutral row-control body on every row: it has no state, so it carries no
// instrument color (the row's identity is on its label swatch).
func TestRowKebabIsNeutral(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false })

	rows := makeTestRows(2)
	rows[0].Color = color.RGBA{190, 120, 60, 255}
	rows[1].Color = color.RGBA{60, 150, 170, 255}

	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 1000, 600))
	tree.Update()
	z.MarkDirty()
	z.Draw(ebiten.NewImage(1000, 600))

	if z.entries[0].menuBtn.Rect().Empty() {
		t.Skip("kebab not laid out at this size")
	}
	for i := range rows {
		if k := styleOf(t, z.entries[i].menuBtn); !colorsEqual(k.Fill, InstButtonStyle.fill()) {
			t.Errorf("row %d kebab fill = %v, want the neutral row-control body %v", i, k.Fill, InstButtonStyle.fill())
		}
	}
}
