//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// After Pads → any audio tab the panel's hit areas must sit where it is drawn: a zone map holding pointers into
// a reallocating slice once laid the panel out against a zero rect, so every tap landed at (0,0).

func makeFullGameTap(g *Game, w, h int) func(mx, my int) {
	return func(mx, my int) {
		var px, py int
		var pressed bool
		restore := SetInputForTest(
			func() (int, int) { return px, py },
			func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return w, h },
		)
		px, py = mx, my
		pressed = true
		g.Update()
		pressed = false
		g.Update()
		restore()
	}
}

var audioViewModes = []struct {
	name string
	mode viewMode
	seg  int
}{
	{"EQ", viewModeEQ, 1},
	{"Wave", viewModeWave, 2},
	{"Spectrum", viewModeSpectrum, 3},
	{"Levels", viewModeMeters, 4},
	{"Chain", viewModeChain, 5},
	{"Synth", viewModeSynth, 6},
}

// Tapping "EQ" from Pads lays the panel out where it is drawn, and a real tap on the channel pill opens the
// dropdown.
func TestEQPanelInput_AliveAfterPadsToEQ(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	const w, h = 360, 700
	g.Layout(w, h)
	advanceFrames(g, 2)
	dv := g.drum

	tap := makeFullGameTap(g, w, h)

	// Pads -> EQ via the segmented control (segment 1), exactly as a user.
	if dv.viewSwitchSegmented == nil {
		t.Fatal("precondition: viewSwitchSegmented non-nil on mobile")
	}
	seg := dv.viewSwitchSegmented.SegmentRect(1)
	tap((seg.Min.X+seg.Max.X)/2, (seg.Min.Y+seg.Max.Y)/2)
	if dv.currentViewMode != viewModeEQ {
		t.Fatalf("precondition: expected viewModeEQ after tapping EQ segment, got %v", dv.currentViewMode)
	}

	panel := dv.eqPanelZone.PanelRect()
	if panel != dv.eqRect {
		t.Fatalf("EQ panel laid out at %v but on-screen rect is %v — controls are not where the user sees them", panel, dv.eqRect)
	}
	if panel.Empty() {
		t.Fatalf("EQ panel rect is empty after Pads->EQ")
	}

	// The channel pill must sit inside the visible panel.
	cr := dv.eqPanelZone.stickyBar.ChannelBtn().Rect()
	if cr.Empty() {
		t.Fatal("channel pill rect is empty after Pads->EQ")
	}
	if !cr.In(panel) {
		t.Fatalf("channel pill rect %v is outside the visible panel %v — its hit area is registered where the user can't tap it", cr, panel)
	}

	// And a real tap where the pill is drawn must open the dropdown.
	if dv.eqPanelZone.ChannelDropdownOpen() {
		dv.eqPanelZone.CloseChannelDropdown()
	}
	tap((cr.Min.X+cr.Max.X)/2, (cr.Min.Y+cr.Max.Y)/2)
	if !dv.eqPanelZone.ChannelDropdownOpen() {
		t.Fatalf("tapping the channel pill at its drawn location (%v) did not open the dropdown — EQ input is dead after Pads->EQ", cr)
	}
}

// Every audio tab, not just EQ.
func TestAudioPanelInput_AliveAfterPadsToTab_AllTabs(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	const w, h = 360, 700

	for _, tc := range audioViewModes {
		t.Run(tc.name, func(t *testing.T) {
			// A fresh game per tab, so no transition inherits another's state.
			logger := game_log.New(testLogOutput(), game_log.LevelError)
			g := New(logger)
			t.Cleanup(g.CloseForTest)
			g.Layout(w, h)
			advanceFrames(g, 2)
			dv := g.drum
			tap := makeFullGameTap(g, w, h)

			// Precondition: a fresh mobile game opens on the Pads view.
			if dv.currentViewMode != viewModeRows {
				t.Fatalf("precondition: fresh mobile game should open on Pads, got %v", dv.currentViewMode)
			}

			// Synth/Sampler need a single instrument (Master disables them).
			if tc.mode == viewModeSynth || tc.mode == viewModeSampler {
				if len(dv.Rows) > 0 {
					dv.eqPanelZone.SetActiveChannel(channel.Of(dv.Rows[0].Instrument))
				}
			}

			// The Pads → tab tap is what triggers the "became visible" re-layout.
			seg := dv.viewSwitchSegmented.SegmentRect(tc.seg)
			tap((seg.Min.X+seg.Max.X)/2, (seg.Min.Y+seg.Max.Y)/2)
			if dv.currentViewMode != tc.mode {
				t.Fatalf("expected %v, got %v", tc.mode, dv.currentViewMode)
			}

			panel := dv.eqPanelZone.PanelRect()
			if panel.Empty() {
				t.Fatalf("[%s] panel rect empty after Pads->%s", tc.name, tc.name)
			}
			if panel != dv.eqRect {
				t.Fatalf("[%s] panel laid out at %v but on-screen rect is %v", tc.name, panel, dv.eqRect)
			}

			// No hit area above the panel, the stale-origin signature; overflow below it is a separate concern.
			for _, ha := range dv.eqPanelZone.HitAreas() {
				if ha.Rect.Empty() {
					continue
				}
				if ha.Rect.Max.Y < panel.Min.Y {
					t.Fatalf("[%s] hit area %q at %v is entirely above the visible panel %v — input registered at the stale origin where the user can't tap",
						tc.name, ha.Tag, ha.Rect, panel)
				}
			}

			cr := dv.eqPanelZone.stickyBar.ChannelBtn().Rect()
			if cr.Empty() {
				t.Fatalf("[%s] channel pill rect empty", tc.name)
			}
			if !cr.In(panel) {
				t.Fatalf("[%s] channel pill %v outside visible panel %v", tc.name, cr, panel)
			}
			if dv.eqPanelZone.ChannelDropdownOpen() {
				dv.eqPanelZone.CloseChannelDropdown()
			}
			tap((cr.Min.X+cr.Max.X)/2, (cr.Min.Y+cr.Max.Y)/2)
			if !dv.eqPanelZone.ChannelDropdownOpen() {
				t.Fatalf("[%s] tapping channel pill at %v did not open dropdown — input dead on this tab", tc.name, cr)
			}
			dv.eqPanelZone.CloseChannelDropdown()
		})
	}
}

// Later RegisterZone appends reallocate t.zones, so zoneMap must not point into it: SetZoneRect on an early zone
// must still reach the live entry.
func TestDrumViewTree_SetZoneRectSurvivesLaterRegistrations(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	first := newTestZone("first")
	// Enough to grow the slice (cap 1→2→4→8) and reallocate.
	tree.RegisterZone(first, 100)
	tree.RegisterZone(newTestZone("b"), 110)
	tree.RegisterZone(newTestZone("c"), 120)
	tree.RegisterZone(newTestZone("d"), 130)
	tree.RegisterZone(newTestZone("e"), 140)

	want := image.Rect(0, 546, 360, 655)
	tree.SetZoneRect("first", want)
	first.lastRect = image.Rectangle{}
	first.needsLayout = false
	tree.Update()

	if first.lastRect != want {
		t.Fatalf("first zone Layout got rect %v, want %v — SetZoneRect lost to a stale zoneMap pointer after slice realloc", first.lastRect, want)
	}
}
