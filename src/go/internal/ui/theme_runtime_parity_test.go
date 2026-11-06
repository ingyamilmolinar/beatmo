//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// runtimeParityScenes are the surfaces a user is likely to have on screen
// when picking a preset in the Theme editor. "" is the plain startup screen.
var runtimeParityScenes = []string{
	"", "eq_tab_synth", "eq_tab_chain", "eq_tab_levels", "eq_tab_wave", "eq_tab_spectrum",
	"node_sidebar_open", "instrument_menu_open", "context_menu_open",
	"fx_panel_with_3_effects", "row_muted_soloed", "transport_playing", "nav_menu_open",
	"crop_sampler_tab_captured", "group_menu_open",
}

// renderPresetFrame renders one scene under a preset. startup=true seeds the
// preset before the first frame (ApplyStoredTheme at boot); startup=false
// warms every cache under the default theme first, then picks the preset the
// way the editor does (Game.setUserThemePreset → OnThemeChanged).
func renderPresetFrame(t *testing.T, presetID, scene string, startup bool) *ebiten.Image {
	return renderPresetFrameSized(t, presetID, scene, startup, 1280, 720, false)
}

// renderPresetFrameSized is renderPresetFrame at an explicit window size;
// mobile=true runs the scene's MobileSetup (portrait phone layout).
func renderPresetFrameSized(t *testing.T, presetID, scene string, startup bool, w, h int, mobile bool) *ebiten.Image {
	t.Helper()
	ApplyTheme(theme.Doc{})
	// Insert effects live in a package-global registry (audio); the FX scene
	// APPENDS three, so each rendered Game starts from an empty chain.
	audio.ClearAllInsertEffects()
	t.Cleanup(audio.ClearAllInsertEffects)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	if startup {
		g.setUserThemePreset(presetID)
	}
	g.Layout(w, h)
	advanceFrames(g, 3)
	screen := ebiten.NewImage(w, h)
	if scene != "" {
		run := RunScene
		if mobile {
			run = RunSceneMobile
		}
		if err := run(g, scene); err != nil {
			t.Fatalf("scene %q: %v", scene, err)
		}
	}
	// Both paths run the SAME number of frames after the scene so animated
	// state (slider easing, the record pulse) is sampled at the same tick;
	// only the runtime path picks the preset between the two draws.
	advanceFrames(g, 4)
	g.Draw(screen)
	if !startup {
		g.setUserThemePreset(presetID)
	}
	// Enough frames for eased slider thumbs and scrollbar fades to settle:
	// a theme change re-lays out the panels, which restarts those animations.
	advanceFrames(g, 40)
	g.Draw(screen)
	return screen
}

type pixelPair struct{ a, b color.RGBA }

// runtimeParityMobileScenes are the mobile surfaces checked at 390×844.
var runtimeParityMobileScenes = []string{"mobile_default", "mobile_bottom_nav_synth", "mobile_bottom_nav_eq", "mobile_overflow_open"}

// diffFrames reports the differing pixel count, bounding box and the most
// common (startup, runtime) color pairs.
func diffFrames(want, got *ebiten.Image, w, h int) (int, image.Rectangle, string) {
	diff := 0
	bbox := image.Rectangle{}
	pairs := map[pixelPair]int{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			b := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			if a == b {
				continue
			}
			diff++
			pairs[pixelPair{a, b}]++
			bbox = bbox.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	type kv struct {
		k pixelPair
		n int
	}
	var top []kv
	for k, n := range pairs {
		top = append(top, kv{k, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	if len(top) > 4 {
		top = top[:4]
	}
	msg := ""
	for _, e := range top {
		msg += fmt.Sprintf("\n  startup #%02X%02X%02X vs runtime #%02X%02X%02X ×%d", e.k.a.R, e.k.a.G, e.k.a.B, e.k.b.R, e.k.b.G, e.k.b.B, e.n)
	}
	return diff, bbox, msg
}

// TestRuntimeThemePickMatchesStartup is the pixel-level guard for
// rebuild-in-place theming: picking a preset at runtime must produce the
// SAME frame as booting with that preset. Any difference is a color captured
// or cached somewhere that OnThemeChanged does not reach (grid subdivision
// styles and the add-row sprite cache were the first two it caught).
func TestRuntimeThemePickMatchesStartup(t *testing.T) {
	assertDefaultParityState(t)
	// The REAL cache path with faithful stub compositing: RENDER_SAFE bypassed
	// the caches where theme drift lives, and without faithful compositing
	// nothing drawn through a zone's clipped sub-image reached the frame — the
	// test passed while the transport kept its old colors after a preset pick.
	withFaithfulCompositing(t)

	for _, p := range themePresets {
		for _, scene := range runtimeParityScenes {
			name := p.ID + "/" + scene
			if scene == "" {
				name = p.ID + "/default"
			}
			t.Run(name, func(t *testing.T) {
				want := renderPresetFrame(t, p.ID, scene, true)
				got := renderPresetFrame(t, p.ID, scene, false)
				if diff, bbox, msg := diffFrames(want, got, 1280, 720); diff != 0 {
					t.Errorf("%d pixels differ between boot-with-preset and runtime pick (bbox %v):%s", diff, bbox, msg)
				}
			})
		}
		for _, scene := range runtimeParityMobileScenes {
			t.Run(p.ID+"/"+scene, func(t *testing.T) {
				want := renderPresetFrameSized(t, p.ID, scene, true, 390, 844, true)
				got := renderPresetFrameSized(t, p.ID, scene, false, 390, 844, true)
				if diff, bbox, msg := diffFrames(want, got, 390, 844); diff != 0 {
					t.Errorf("%d pixels differ between boot-with-preset and runtime pick (bbox %v):%s", diff, bbox, msg)
				}
			})
		}
	}
}
