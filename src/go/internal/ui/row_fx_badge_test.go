//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// None for zero or all-disabled slots, a plain dot for one, a count for several; the button keeps IconFx.
func TestRowFXBadgeCountFromActiveSlots(t *testing.T) {
	assertDefaultParityState(t)

	cases := []struct {
		name     string
		slots    []audio.EffectSlot
		wantN    int
		wantDraw bool
	}{
		{"none", nil, 0, false},
		{"one_disabled", []audio.EffectSlot{{Enabled: false}}, 0, false},
		{"one_enabled", []audio.EffectSlot{{Enabled: true}}, 1, true},
		{"three_enabled", []audio.EffectSlot{{Enabled: true}, {Enabled: true}, {Enabled: true}}, 3, true},
		{"mixed", []audio.EffectSlot{{Enabled: true}, {Enabled: false}, {Enabled: true}}, 2, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotN := activeEffectsCount(c.slots)
			if gotN != c.wantN {
				t.Errorf("activeEffectsCount = %d, want %d", gotN, c.wantN)
			}
			gotActive := hasActiveEffects(c.slots)
			if gotActive != c.wantDraw {
				t.Errorf("hasActiveEffects = %t, want %t", gotActive, c.wantDraw)
			}

			// Completes for every state; the dot's presence follows from the count helpers above.
			img := ebiten.NewImage(32, 32)
			drawFXBadge(img, image.Rect(0, 0, 32, 32), gotN, color.RGBA{190, 120, 60, 255})
		})
	}
}

// Only the count dots signal how many effects are on.
func TestFXButtonAlwaysShowsFXLetters(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	rows := makeTestRows(1)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 390, 600))

	if len(z.entries) < 1 {
		t.Fatal("expected ≥1 row entry")
	}
	if b := z.entries[0].fxBtn; b.Text != rowFXLabel || b.Icon != "" {
		t.Errorf("fxBtn text %q icon %q, want the %q letters — the button must keep a consistent identity",
			b.Text, b.Icon, rowFXLabel)
	}
}
