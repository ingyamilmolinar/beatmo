//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestRowFXBadgeCountFromActiveSlots verifies the FX badge helper interprets
// the slot list correctly:
//   - zero or all-disabled slots → no badge (count 0).
//   - one enabled slot → badge with no numeric label.
//   - multiple enabled slots → badge with numeric count text.
//
// The badge replaces the previous icon-swap convention so the FX button
// always shows IconFx; the count surfaces "how many" without changing the
// button's identity.
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

			// Render a 32x32 button rect through drawFXBadge and confirm the
			// helper completes without panic for every state. The visual
			// presence/absence of the dot is asserted indirectly via the
			// count helpers above; pixel-level verification lives in the
			// regenerated mobile screenshot baseline.
			img := ebiten.NewImage(32, 32)
			drawFXBadge(img, image.Rect(0, 0, 32, 32), gotN, color.RGBA{190, 120, 60, 255})
		})
	}
}

// TestFXButtonAlwaysShowsIconFx ensures the FX button keeps its identity
// across active/inactive states — only the badge layer signals the count.
func TestFXButtonAlwaysShowsIconFx(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	rows := makeTestRows(1)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 390, 600))

	if len(z.entries) < 1 {
		t.Fatal("expected ≥1 row entry")
	}
	if got := z.entries[0].fxBtn.Icon; got != string(IconFx) {
		t.Errorf("fxBtn.Icon = %q, want IconFx (%q) — the button must keep a consistent identity",
			got, string(IconFx))
	}
}
