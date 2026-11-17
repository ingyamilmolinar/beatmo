//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Every pressable control shares ONE body: the spec keycap fill (surface-2)
// drawn by Render/ButtonStyle.Draw. Two families used to deviate — the
// volume buttons painted no body at all (the bed showed through), and text
// inputs painted a darker "recessed" surface-1 slot — so the master volume
// button, the BPM box and the EQ dB boxes read as different components from
// the keycaps beside them. Guard: both now paint the same body as a button.
func TestVolumeButtonsAndInputsShareKeycapBody(t *testing.T) {
	want := Spec(ComponentButtonSecondary).Fill
	r := image.Rect(10, 10, 42, 42)

	dst := ebiten.NewImage(64, 64)
	calls := captureRoundedRect(t, func() { drawVolumeButton(dst, r, 0.5, false, nil) })
	found := false
	for _, c := range calls {
		if c.Filled && c.Rect == r && colorEqRGB(c.Color, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("drawVolumeButton painted no keycap body %v at %v: %+v", want, r, calls)
	}

	in := TextInputStyleFromSpec(ComponentInputField, color.White)
	calls = captureRoundedRect(t, func() { in.Draw(dst, r, false, false) })
	found = false
	for _, c := range calls {
		if c.Filled && c.Rect == r && colorEqRGB(c.Color, Spec(ComponentInputField).Fill) {
			found = true
		}
		if c.Filled && colorEqRGB(c.Color, colSurface1) {
			t.Fatalf("text input still paints the recessed surface-1 slot: %+v", c)
		}
	}
	if !found {
		t.Fatalf("text input body must be the input-field spec fill %v (same tone as the keycaps): %+v", Spec(ComponentInputField).Fill, calls)
	}
	if Spec(ComponentInputField).Fill != want {
		t.Fatalf("input-field spec fill %v differs from button-secondary %v — DESIGN.md components drifted apart", Spec(ComponentInputField).Fill, want)
	}
}
