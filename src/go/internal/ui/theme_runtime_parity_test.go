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

// What a user likely has on screen when picking a preset; "" is the plain startup screen.
var runtimeParityScenes = []string{
	"", "eq_tab_synth", "eq_tab_chain", "eq_tab_levels", "eq_tab_wave", "eq_tab_spectrum",
	"node_sidebar_open", "instrument_menu_open", "context_menu_open",
	"fx_panel_with_3_effects", "row_muted_soloed", "transport_playing", "nav_menu_open",
	"crop_sampler_tab_captured", "group_menu_open",
}

// startup seeds the preset before the first frame; otherwise caches warm under the default theme first and the
// preset is picked the way the editor does.
func renderPresetFrame(t *testing.T, presetID, scene string, startup bool) *ebiten.Image {
	return renderPresetFrameSized(t, presetID, scene, startup, 1280, 720, false)
}

func renderPresetFrameSized(t *testing.T, presetID, scene string, startup bool, w, h int, mobile bool) *ebiten.Image {
	t.Helper()
	ApplyTheme(theme.Doc{})
	// Insert effects are a package-global registry and the FX scene APPENDS to it.
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
	// The same frame count on both paths, so animated state is sampled at the same tick.
	advanceFrames(g, 4)
	g.Draw(screen)
	if !startup {
		g.setUserThemePreset(presetID)
	}
	// A theme change re-lays out the panels, restarting eased thumbs and fades.
	advanceFrames(g, 40)
	g.Draw(screen)
	return screen
}

type pixelPair struct{ a, b color.RGBA }

// runtimeParityMobileScenes are the mobile surfaces checked at 390×844.
var runtimeParityMobileScenes = []string{"mobile_default", "mobile_bottom_nav_synth", "mobile_bottom_nav_eq", "mobile_overflow_open"}

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

// Any difference is a colour captured or cached somewhere OnThemeChanged does not reach.
func TestRuntimeThemePickMatchesStartup(t *testing.T) {
	assertDefaultParityState(t)
	// The REAL cache path with faithful compositing: RENDER_SAFE bypasses the caches where drift lives, and without
	// faithful compositing nothing drawn through a clipped sub-image reaches the frame.
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
