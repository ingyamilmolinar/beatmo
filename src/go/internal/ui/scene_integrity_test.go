//go:build test

package ui

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A variant scene must render differently from its base: a Setup that flips a flag on a path that never renders
// captures a PNG identical to the base. drawRect digests, not pixels: under the stub sprite-cached content copies
// one pixel and SubImage draws land elsewhere, while drawRect sees every rect the renderer emits.

// The first scene drawn in a process emits one-time warm-up rects, which would make it look different for the
// wrong reason.
var sceneDigestWarmup sync.Once

// Taller than 720 on purpose: at 720 the EQ gain plot is below drawEQCurve's floor, so EQ variants could not show
// their state.
const sceneDigestWidth, sceneDigestHeight = 1280, 900

// BPM is folded in: a BPM-only change renders as a glyph the stub can't diff.
func renderSceneDigest(t *testing.T, name string, w, h int) [32]byte {
	t.Helper()
	sceneDigestWarmup.Do(func() {
		wg := New(testLogger)
		defer wg.CloseForTest()
		wg.Layout(w, h)
		_ = RunScene(wg, "transport_idle")
		for i := 0; i < 8; i++ {
			_ = wg.Update()
		}
		wg.Draw(ebiten.NewImage(w, h))
	})

	// A clean audio baseline, so a previous scene's HPF or analyzer state can't bleed in.
	audio.Reset()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)

	if err := RunScene(g, name); err != nil {
		t.Fatalf("RunScene(%q): %v", name, err)
	}
	for i := 0; i < 8; i++ {
		_ = g.Update()
	}

	screen := ebiten.NewImage(w, h)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})

	hsh := sha256.New()
	var scratch [4]byte
	put := func(v int) {
		binary.LittleEndian.PutUint32(scratch[:], uint32(v))
		hsh.Write(scratch[:])
	}
	for _, r := range rects {
		put(r.Rect.Min.X)
		put(r.Rect.Min.Y)
		put(r.Rect.Max.X)
		put(r.Rect.Max.Y)
		put(int(r.Color.R))
		put(int(r.Color.G))
		put(int(r.Color.B))
		put(int(r.Color.A))
	}
	if g.drum != nil {
		put(g.drum.BPM())
	}
	var out [32]byte
	copy(out[:], hsh.Sum(nil))
	return out
}

func TestSceneVariantsDifferFromBase(t *testing.T) {
	assertDefaultParityState(t)

	const w, h = sceneDigestWidth, sceneDigestHeight

	cases := []struct {
		variant string
		base    string
		skip    string // non-empty => t.Skip with this reason
	}{
		{variant: "eq_hpf_active", base: "eq_tab_eq"},
		{variant: "eq_lpf_active", base: "eq_tab_eq"},
		{variant: "eq_band_muted", base: "eq_tab_eq"},
		{variant: "eq_with_band_adjusted", base: "eq_tab_eq"},
		{variant: "eq_chain_custom_settings", base: "eq_tab_chain"},
		{variant: "transport_high_bpm", base: "transport_idle"},
		{variant: "transport_playing", base: "transport_idle",
			skip: "playback's visible deltas (beat highlights, play→pause icon) are sprite/glyph draws the stub renders as single-pixel blits, so they don't surface in the drawRect digest; SetPlaying state is exercised by playback tests elsewhere"},
		{variant: "multi_row_full_grid", base: "transport_idle"},
		{variant: "crop_drum_view_multi_row", base: "crop_drum_view_default"},
		{variant: "crop_drum_rows_multi_row", base: "crop_drum_rows_default"},
		{variant: "node_added", base: "transport_idle"},
		{variant: "graph_complex_3_nodes_4_edges", base: "transport_idle"},
	}

	// A rename must not silently drop a guard entry.
	known := map[string]bool{}
	for _, s := range sceneCatalog {
		known[s.Name] = true
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.variant, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			if !known[tc.variant] {
				t.Fatalf("variant scene %q not in catalog", tc.variant)
			}
			if !known[tc.base] {
				t.Fatalf("base scene %q not in catalog", tc.base)
			}
			vd := renderSceneDigest(t, tc.variant, w, h)
			bd := renderSceneDigest(t, tc.base, w, h)
			if vd == bd {
				t.Errorf("scene %q renders byte-identical to base %q — its Setup applied no visible state",
					tc.variant, tc.base)
			}
		})
	}
}

// Placing nodes selects them and arms the coordinate badge; the Setup clears it so it never reaches a screenshot.
func TestNoCoordBadgeLeaksIntoGraphScenes(t *testing.T) {
	assertDefaultParityState(t)
	for _, name := range []string{
		"node_added",
		"edge_built",
		"graph_complex_3_nodes_4_edges",
		"crop_main_grid_with_3_nodes",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			if err := RunScene(g, name); err != nil {
				t.Fatalf("RunScene(%q): %v", name, err)
			}
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}
			if g.coordBadgeNode != nil {
				t.Errorf("scene %q left coordBadgeNode armed — the coord pill leaks into the capture", name)
			}
		})
	}
}

// Otherwise it captures the plain synth-tab crop.
func TestCropMasterChooserOpensChooser(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := RunScene(g, "crop_synth_tab_master_chooser"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	if g.drum == nil || g.drum.eqPanelZone == nil {
		t.Fatal("eq panel zone unreachable")
	}
	// Right after Setup, with no Update frames: leaked mock cursor state from a sibling test can synthesise an outside
	// click that closes the portal.
	if !g.drum.eqPanelZone.ChannelDropdownOpen() {
		t.Error("crop_synth_tab_master_chooser did not open the channel chooser dropdown")
	}
}

// The same Setup+Draw path the harness uses.
func TestEqTabSynthCapturesWithoutPanic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := RunScene(g, "eq_tab_synth"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = g.Update()
	}
	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen) // must not panic
}
