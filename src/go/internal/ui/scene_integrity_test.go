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

// scene_integrity_test.go — the variant→base guard.
//
// Several "variant" scenes in the catalog are meant to capture a state
// that is VISIBLY different from a base scene (HPF engaged, a band muted,
// a high BPM, more rows, …). A bug class we keep hitting is a Setup func
// that flips a flag on a path that never renders — the captured PNG ends
// up byte-identical to the base scene, so the "variant" screenshot is a
// lie that no bounds-only check (scene_crop_test.go) catches.
//
// This guard renders each variant scene AND its declared base to an
// ebitenstub framebuffer, builds a digest from every drawRect call emitted
// during Draw (the canonical render-content signal — see
// audio_panel_render_pixels_test.go / render_verify_test.go), and asserts
// the two digests DIFFER. If a Setup silently no-ops, the digests match
// and the test fails — exactly the signal the audit needed.
//
// Why drawRect digests and not raw pixels: under the test stub,
// DrawImage of sprite-cached content (nodes, glyphs) copies only the
// first pixel, and SubImage draws land in an independent buffer, so a
// raw framebuffer hash misses most content. drawRect is intercepted at
// the package-level var and captures every filled/stroked rect + color
// the renderer emits, regardless of sprite caching.

// sceneDigestWarmup runs one throwaway render so the process-global text
// sprite cache and first-frame full-repaint are populated before any
// measured render. Without it the FIRST scene drawn in the process emits
// extra one-time warmup rects, which would make whichever variant/base
// rendered first look "different" for the wrong reason (a false negative
// that masks a real Setup no-op, or a false positive that hides a match).
var sceneDigestWarmup sync.Once

// sceneDigestHeight is taller than the 1280x720 capture window on purpose:
// at 720p the EQ panel's gain-plot collapses to ~14px (below drawEQCurve's
// 16px floor), so the curve never draws and EQ-gain variants can't show
// their distinguishing state. Rendering the guard at a height where the
// plot is tall enough to draw the curve lets the guard observe that the
// Setup actually applied gains/filters — the layout-cramping at 720p is a
// separate, known constraint of the capture window itself.
const sceneDigestWidth, sceneDigestHeight = 1280, 900

// renderSceneDigest applies the named scene, settles a few frames, draws
// to a fresh framebuffer, and returns a content digest over every drawRect
// call plus the BPM (BPM-only changes render as glyphs the stub can't
// pixel-diff, so we fold it into the digest explicitly).
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

	// Each scene starts from a clean global audio baseline so a prior
	// scene's HPF/analyzer state can't bleed into this render.
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
	// Fold BPM in: a BPM-only variant changes a glyph the stub renders as a
	// near-transparent sprite that won't show up in drawRect output, so the
	// digest would otherwise miss it.
	if g.drum != nil {
		put(g.drum.BPM())
	}
	var out [32]byte
	copy(out[:], hsh.Sum(nil))
	return out
}

// TestSceneVariantsDifferFromBase — every variant scene must render a
// content digest distinct from its declared base scene. A match means the
// variant's Setup silently failed to apply its distinguishing state.
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

	// Sanity: every variant/base referenced here must exist in the catalog,
	// so a rename can't silently drop a guard entry.
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

// TestNoCoordBadgeLeaksIntoGraphScenes — the graph scenes place nodes,
// which selects them and arms the coordinate badge ("(i, j)" pill). That
// debug-ish hover affordance must not leak into a screenshot: the scene
// Setup clears g.coordBadgeNode after placement. This pins the
// suppression so a future Setup that re-arms the badge is caught.
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

// TestCropMasterChooserOpensChooser — crop_synth_tab_master_chooser must
// actually open the channel chooser surface; otherwise it captures the
// same thing as the plain synth-tab crop and the name is a lie.
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
	// Assert right after Setup: the chooser must be open as a direct result
	// of the scene's Setup. We deliberately do NOT drive Update frames here —
	// a sibling test that leaks ebiten mock cursor/touch state can synthesize
	// an outside-click during Update that auto-closes the portal, which is an
	// input-isolation concern of those tests, not a defect of this scene's
	// Setup. (The chooser does survive settle frames in isolation.)
	if !g.drum.eqPanelZone.ChannelDropdownOpen() {
		t.Error("crop_synth_tab_master_chooser did not open the channel chooser dropdown")
	}
}

// TestEqTabSynthCapturesWithoutPanic — the plain (non-crop) Synth EQ tab
// scene must apply + draw without panicking. The audit reported it as the
// one outright capture failure; this exercises the same Setup+Draw path
// the harness uses.
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
