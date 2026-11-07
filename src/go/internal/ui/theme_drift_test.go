//go:build test

package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// Over EVERY scene, with faithful compositing (else clipped sub-image draws are lost): a runtime theme switch gives
// the boot frame on the real cache path, and a colour surviving a loud theme AND a shifted instrument palette is
// hard-coded unless it is a fixed token or black/white.

// loudThemeCategories moves every theme category far from the default.
var loudThemeCategories = map[string]string{
	"background": "mint-500", "surfaces": "cyan-500", "text": "sunset-gold-100",
	"accent": "electric-blue-200", "focus": "mint-300", "grid": "tangerine-500",
	"nodes": "cyan-500", "dividers": "mint-100", "viz": "hot-pink-300",
}

// Each with its reason.
var themeDriftExempt = map[string]string{
	"theme_editor_picker": "the swatch picker's keyboard cursor is placed on the current swatch when it OPENS — navigation state seeded before the runtime switch, not a color",
}

func applyLoudTheme(g *Game) {
	for c, sw := range loudThemeCategories {
		g.setUserThemeCategory(c, sw)
	}
}

// A scene that sets the theme itself ends its two paths on different themes by construction; presets have their own
// parity test.
func renderThemeSwitchFrame(t *testing.T, scene string, boot, mobile bool, w, h int) (img *ebiten.Image, sceneSetTheme bool) {
	t.Helper()
	// Restored when THIS render returns, or the next render in the subtest boots in another profile.
	defer restoreScreenProfile(forceSmallScreenForTest)
	ApplyTheme(theme.Doc{})
	audio.ClearAllInsertEffects()
	t.Cleanup(audio.ClearAllInsertEffects)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { stopSceneSideEffects(g) })
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	if boot {
		applyLoudTheme(g)
	}
	g.Layout(w, h)
	advanceFrames(g, 3)
	screen := ebiten.NewImage(w, h)
	gen := ThemeGeneration()
	if scene != "" {
		run := RunScene
		if mobile {
			run = RunSceneMobile
		}
		if err := run(g, scene); err != nil {
			t.Fatalf("scene %q: %v", scene, err)
		}
	}
	advanceFrames(g, 4)
	sceneSetTheme = ThemeGeneration() != gen
	settleAsyncForCapture(g)
	g.Draw(screen)
	if !boot {
		applyLoudTheme(g)
	}
	// Enough frames for eased animations restarted by the re-layout to settle.
	advanceFrames(g, 40)
	settleAsyncForCapture(g)
	g.Draw(screen)
	return screen, sceneSetTheme
}

func themeDriftScenes(mobile bool) []string {
	var out []string
	for _, s := range sceneCatalog {
		if mobile && !s.Mobile {
			continue
		}
		out = append(out, s.Name)
	}
	return out
}

func TestRuntimeThemeSwitchMatchesBootEveryScene(t *testing.T) {
	assertDefaultParityState(t)
	withFaithfulCompositing(t)
	defer SetThemeSink(nil)
	SetThemeSink(&fakeThemeSink{})
	for _, pass := range []struct {
		name   string
		mobile bool
		w, h   int
	}{{"desktop", false, 1280, 720}, {"mobile", true, 390, 844}} {
		for _, scene := range themeDriftScenes(pass.mobile) {
			t.Run(pass.name+"/"+scene, func(t *testing.T) {
				if why, ok := themeDriftExempt[scene]; ok {
					t.Skip(why)
				}
				want, setA := renderThemeSwitchFrame(t, scene, true, pass.mobile, pass.w, pass.h)
				got, setB := renderThemeSwitchFrame(t, scene, false, pass.mobile, pass.w, pass.h)
				if setA || setB {
					t.Skip("scene sets the theme itself (covered by TestRuntimeThemePickMatchesStartup)")
				}
				if diff, bbox, msg := diffFrames(want, got, pass.w, pass.h); diff != 0 {
					t.Errorf("%d pixels differ between booting with the theme and switching to it at runtime (bbox %v) — "+
						"a cache or captured color does not follow a theme change:%s", diff, bbox, msg)
				}
			})
		}
	}
}

// ── color provenance ────────────────────────────────────────────────────

// Instrument colours are left alone by a theme, so they are shifted to change between the two renders too.
func shiftInstrumentColor(c color.Color) color.Color {
	if c == nil {
		return nil
	}
	r := color.RGBAModel.Convert(c).(color.RGBA)
	return color.RGBA{R: r.G ^ 0x2D, G: r.B ^ 0x2D, B: r.R ^ 0x2D, A: r.A}
}

// Call it AFTER the theme is applied: rebuildThemeAliases rebuilds the instrument tables and would undo an earlier
// shift.
func shiftInstrumentPalette() func() {
	oldInst, oldCustom, oldAssigned := instColors, customPalette, customColors
	oldSeq, oldSw := genInstrumentSequence, SuggestedInstrumentSwatches
	ni := make(map[instrument.ID]color.Color, len(instColors))
	for k, v := range instColors {
		ni[k] = shiftInstrumentColor(v)
	}
	// customColors is package-global and outlives a Game.
	na := make(map[instrument.ID]color.Color, len(customColors))
	for k, v := range customColors {
		na[k] = shiftInstrumentColor(v)
	}
	customColors = na
	nc := make([]color.Color, len(customPalette))
	for i, v := range customPalette {
		nc[i] = shiftInstrumentColor(v)
	}
	ns := make([]color.RGBA, len(genInstrumentSequence))
	for i, v := range genInstrumentSequence {
		ns[i] = shiftInstrumentColor(v).(color.RGBA)
	}
	nw := make([]GenInstrumentSwatch, len(SuggestedInstrumentSwatches))
	for i, v := range SuggestedInstrumentSwatches {
		v.RGBA = shiftInstrumentColor(v.RGBA).(color.RGBA)
		nw[i] = v
	}
	instColors, customPalette, genInstrumentSequence, SuggestedInstrumentSwatches = ni, nc, ns, nw
	return func() {
		instColors, customPalette, customColors = oldInst, oldCustom, oldAssigned
		genInstrumentSequence, SuggestedInstrumentSwatches = oldSeq, oldSw
	}
}

type colorSite struct {
	n     int
	stack string
}

// Alpha is a strength, not an identity: WithAlpha(token, a) is still the token.
func drawnColorKey(c color.RGBA) (color.RGBA, bool) {
	if c.A == 0 {
		return color.RGBA{}, false
	}
	un := func(v uint8) uint8 { return uint8((uint32(v)*255 + uint32(c.A)/2) / uint32(c.A)) }
	return color.RGBA{R: un(c.R), G: un(c.G), B: un(c.B), A: 255}, true
}

// The call site that CHOSE the colour.
func provenanceStack() string {
	pcs := make([]uintptr, 24)
	n := runtime.Callers(3, pcs)
	fr := runtime.CallersFrames(pcs[:n])
	primitive := map[string]bool{
		"drawing.go": true, "button_flat.go": true, "textcache.go": true, "icons.go": true,
		"iconsprite.go": true, "icon_renderer.go": true, "render.go": true, "trace_sprite.go": true,
		"ebiten.go": true, "theme_drift_test.go": true, "fontcache.go": true, "image_metrics.go": true,
		"drawing_icons.go": true, "nodesprite.go": true,
	}
	var parts []string
	for len(parts) < 4 {
		f, more := fr.Next()
		base := filepath.Base(f.File)
		if strings.Contains(f.File, "/internal/ui/") && !primitive[base] {
			parts = append(parts, fmt.Sprintf("%s:%d", base, f.Line))
		}
		if !more {
			break
		}
	}
	return strings.Join(parts, " ← ")
}

// Fills by their pixel blit's colour, text and icons by their tint.
func recordDrawnColors(t *testing.T, scene string, loud, mobile bool) (map[color.RGBA]*colorSite, bool) {
	t.Helper()
	defer restoreScreenProfile(forceSmallScreenForTest)
	// Restored when THIS render returns: a palette shift leaking into the next base render poisons the diff.
	ApplyTheme(theme.Doc{})
	audio.ClearAllInsertEffects()
	defer audio.ClearAllInsertEffects()
	if loud {
		// Imported rows take their colour from the file and constructed rows from the palette tables: both are shifted before
		// New.
		importedRowColorTransform = shiftInstrumentColor
		defer func() { importedRowColorTransform = nil }()
		defer shiftInstrumentPalette()()
	}
	g := New(testLogger)
	defer g.CloseForTest()
	defer stopSceneSideEffects(g)
	defer ApplyTheme(theme.Doc{})
	if loud {
		applyLoudTheme(g)
		defer shiftInstrumentPalette()()
	}
	w, h := 1280, 720
	if mobile {
		w, h = 390, 844
	}
	g.Layout(w, h)
	advanceFrames(g, 3)
	gen := ThemeGeneration()
	if scene != "" {
		run := RunScene
		if mobile {
			run = RunSceneMobile
		}
		if err := run(g, scene); err != nil {
			t.Fatalf("scene %q: %v", scene, err)
		}
	}
	advanceFrames(g, 4)
	setTheme := ThemeGeneration() != gen
	// Flushes every colour-baking cache.
	g.OnThemeChanged()
	advanceFrames(g, 2)
	settleAsyncForCapture(g)
	got := map[color.RGBA]*colorSite{}
	note := func(c color.RGBA) {
		k, ok := drawnColorKey(c)
		if !ok {
			return
		}
		s := got[k]
		if s == nil {
			s = &colorSite{stack: provenanceStack()}
			got[k] = s
		}
		s.n++
	}
	ebiten.DrawImageObserver = func(dst, src *ebiten.Image, o *ebiten.DrawImageOptions) {
		b := src.Bounds()
		if b.Dx()*b.Dy() == 1 {
			c := color.RGBAModel.Convert(src.At(b.Min.X, b.Min.Y)).(color.RGBA)
			if r, gg, bb, a, ok := o.ColorScale.Factors(); ok {
				c = color.RGBA{R: uint8(float32(c.R) * r * a), G: uint8(float32(c.G) * gg * a), B: uint8(float32(c.B) * bb * a), A: uint8(float32(c.A) * a)}
			}
			note(c)
			return
		}
		if r, gg, bb, a, ok := o.ColorScale.Factors(); ok && a > 0 {
			cl := func(v float32) uint8 {
				if v > 1 {
					v = 1
				}
				return uint8(v*255 + 0.5)
			}
			note(color.RGBA{R: cl(r * a), G: cl(gg * a), B: cl(bb * a), A: cl(a)})
		}
	}
	defer func() { ebiten.DrawImageObserver = nil }()
	g.Draw(ebiten.NewImage(w, h))
	return got, setTheme
}

// fixedThemeColors are the colors a theme deliberately never changes.
func fixedThemeColors() []color.RGBA {
	out := []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}
	for name := range themeFixedTokens {
		if p, ok := genColorByName[name]; ok {
			c := *p
			c.A = 255
			out = append(out, c)
		}
	}
	return out
}

// A low-alpha draw loses a few RGB steps to premultiplied rounding.
func isFixedColor(c color.RGBA, fixed []color.RGBA) bool {
	near := func(a, b uint8) bool {
		d := int(a) - int(b)
		return d >= -6 && d <= 6
	}
	for _, f := range fixed {
		if near(c.R, f.R) && near(c.G, f.G) && near(c.B, f.B) {
			return true
		}
	}
	return false
}

func TestNoColorRendersOutsideTheTheme(t *testing.T) {
	assertDefaultParityState(t)
	withFaithfulCompositing(t)
	defer SetThemeSink(nil)
	SetThemeSink(&fakeThemeSink{})
	fixed := fixedThemeColors()
	type violation struct {
		col    color.RGBA
		scenes []string
		stack  string
	}
	found := map[color.RGBA]*violation{}
	for _, mobile := range []bool{false, true} {
		for _, scene := range themeDriftScenes(mobile) {
			if _, ok := themeDriftExempt[scene]; ok {
				continue
			}
			base, setA := recordDrawnColors(t, scene, false, mobile)
			loud, setB := recordDrawnColors(t, scene, true, mobile)
			if setA || setB {
				continue // the scene picks its own theme
			}
			name := scene
			if mobile {
				name = "mobile/" + scene
			}
			for c, site := range loud {
				if _, inBase := base[c]; !inBase || isFixedColor(c, fixed) {
					continue
				}
				v := found[c]
				if v == nil {
					v = &violation{col: c, stack: site.stack}
					found[c] = v
				}
				v.scenes = append(v.scenes, name)
			}
		}
	}
	if len(found) == 0 {
		return
	}
	var list []*violation
	for _, v := range found {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return len(list[i].scenes) > len(list[j].scenes) })
	var b strings.Builder
	for _, v := range list {
		ex := v.scenes
		if len(ex) > 3 {
			ex = ex[:3]
		}
		fmt.Fprintf(&b, "\n  #%02X%02X%02X in %d scene(s) %v — drawn at %s", v.col.R, v.col.G, v.col.B, len(v.scenes), ex, v.stack)
	}
	t.Errorf("%d color(s) render identically under a different theme AND a shifted instrument palette — "+
		"they come from neither (a literal, a raw ramp entry, a value captured at init). Route them through a "+
		"theme token, or declare them in themeFixedTokens if they are genuinely semantic:%s", len(list), b.String())
}

// A recording session owns goroutines that goleak checks at TestMain.
func stopSceneSideEffects(g *Game) {
	if g.IsRecording() {
		_, _ = g.StopRecording()
	}
}

func restoreScreenProfile(prev bool) {
	if forceSmallScreenForTest != prev {
		forceSmallScreenForTest = prev
		UpdateProfile()
	}
}

// Shared SubImage views and applied ColorScale tints, as real Ebiten does.
func withFaithfulCompositing(t *testing.T) {
	t.Helper()
	prev := ebiten.FaithfulCompositing
	ebiten.FaithfulCompositing = true
	t.Cleanup(func() { ebiten.FaithfulCompositing = prev })
}

// Settled synchronously so two captures cannot differ by whether a worker finished first.
func settleAsyncForCapture(g *Game) {
	if g == nil || g.drum == nil || g.drum.eqPanelZone == nil {
		return
	}
	if g.drum.eqPanelZone.ActiveTab() == TabSynth || g.drum.currentViewMode == viewModeSynth {
		settleSynthTab(g)
	}
}
