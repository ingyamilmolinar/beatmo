//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// styleOf type-asserts a button's visual to the concrete ButtonStyle so tests
// can read the resolved fill/border. Fails the test if the style isn't a
// ButtonStyle (the per-row toggles all resolve to one).
func styleOf(t *testing.T, b *Button) ButtonStyle {
	t.Helper()
	st, ok := b.Style.(ButtonStyle)
	if !ok {
		t.Fatalf("button style is %T, want ButtonStyle", b.Style)
	}
	return st
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

	// Inactive solo (row 1) is a dim tint, not the same as its active fill.
	solo1 := styleOf(t, z.entries[1].soloBtn)
	if colorsEqual(solo0.Fill, solo1.Fill) {
		t.Error("active and inactive solo fills must differ")
	}
	if a := rowShadeToRGBA(solo1.Fill).A; a >= 200 {
		t.Errorf("inactive solo fill should be a dim (low-alpha) tint, got alpha %d", a)
	}
}

// TestRowKebabFollowsInstrumentColor verifies the per-row ellipsis (kebab) chip
// is tinted with the instrument color instead of the fixed RowKebabChipStyle.
func TestRowKebabFollowsInstrumentColor(t *testing.T) {
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
	k0 := styleOf(t, z.entries[0].menuBtn)
	k1 := styleOf(t, z.entries[1].menuBtn)
	if colorsEqual(k0.Fill, k1.Fill) {
		t.Errorf("kebab fills must follow per-row instrument color: %v vs %v", k0.Fill, k1.Fill)
	}
	// And it must match the row's computed kebab tint.
	want := rowKebabStyle(rows[0].Color)
	if !colorsEqual(k0.Fill, want.Fill) {
		t.Errorf("kebab fill = %v, want instrument tint %v", k0.Fill, want.Fill)
	}
}
