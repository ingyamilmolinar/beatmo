//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// The crop test checks only bounds, so a panel drawing chrome with a blank data path passed everything; this
// injects deterministic analyser data and counts what the renderer draws with it.

// A recognisable sine and a non-flat spectrum, as a live AnalyserNode delivers during playback.
func snapshotWithSine() audio.AnalyzerSnapshot {
	const n = 512
	wave := make([]float64, n)
	for i := range wave {
		wave[i] = 0.6 * math.Sin(2*math.Pi*float64(i)/64.0)
	}
	spec := make([]float64, 64)
	for i := range spec {
		// Decaying, so the FFT bins vary instead of sitting at the -80 dB floor.
		spec[i] = 0.9 * math.Exp(-float64(i)/12.0)
	}
	return audio.AnalyzerSnapshot{
		RMS:      0.42,
		Peak:     0.6,
		Spectrum: spec,
		Waveform: wave,
	}
}

// Data colours tell data from chrome; counting rects rather than coverage avoids overlap double-counting.
func rectsWithColorInside(rects []drawnRect, inside image.Rectangle, colors ...color.Color) int {
	if inside.Empty() {
		return 0
	}
	wanted := make(map[color.RGBA]bool, len(colors))
	for _, c := range colors {
		wanted[color.RGBAModel.Convert(c).(color.RGBA)] = true
	}
	count := 0
	for _, r := range rects {
		if !wanted[r.Color] {
			continue
		}
		if r.Rect.Intersect(inside).Empty() {
			continue
		}
		count++
	}
	return count
}

// An empty snapshot draws no meter-colour rect at all, so any count proves the renderer got data.
func barRectsInsideMeter(rects []drawnRect, inside image.Rectangle) int {
	if inside.Empty() {
		return 0
	}
	// WithAlpha returns NRGBA, so the RMS overlays are built the way the renderer builds them.
	wantedRGB := []color.RGBA{meterLow, meterMid, meterHigh}
	wanted := make(map[color.RGBA]bool, 6)
	for _, c := range wantedRGB {
		wanted[c] = true
		// RMS overlay drawn via meterColorAlpha(db, meterRMSOpacity).
		ovl := WithAlpha(c, meterRMSOpacity)
		wanted[color.RGBAModel.Convert(ovl).(color.RGBA)] = true
	}
	count := 0
	for _, r := range rects {
		if !wanted[r.Color] {
			continue
		}
		if r.Rect.Intersect(inside).Empty() {
			continue
		}
		// meterLow shares its gold with the full-width chrome line; meter bars are inset within their strips.
		if r.Rect.Min.X <= inside.Min.X && r.Rect.Max.X >= inside.Max.X {
			continue
		}
		// The 2 px selection bar shares meterLow's gold too; meter bars are taller.
		if r.Rect.Dy() == flatSelectionBarPx {
			continue
		}
		count++
	}
	return count
}

// A few Update frames, as the screenshot harness's settle countdown runs.
func driveScene(t *testing.T, name string) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	if err := RunScene(g, name); err != nil {
		t.Fatalf("RunScene(%q): %v", name, err)
	}
	for i := 0; i < 32; i++ {
		_ = g.Update()
	}
	return g
}

// Injected everywhere the renderer reads: eqTestSnapshot (the Wave fallback), testAnalyzerStateOverride (Spectrum,
// Meters, Wave) and testScopeStateOverride (Scope), all reset on Cleanup.
func drawWithSnapshot(t *testing.T, g *Game, subject Subject, snap audio.AnalyzerSnapshot) (image.Rectangle, []drawnRect) {
	t.Helper()
	if g.drum == nil {
		t.Fatal("game.drum is nil")
	}

	// The trace caches draw at a (0,0) origin; render directly so the rects are counted at the subject position.
	t.Cleanup(SetChainTraceCacheForTest(false))
	t.Cleanup(SetWaveTraceCacheForTest(false))

	// Legacy DrawWaveform fallback path.
	post := snap
	pre := snap
	g.drum.eqTestSnapshot = &post
	g.drum.eqTestPreEQSnapshot = &pre

	// AnalyzerState path (Spectrum, Meters, also Wave on WASM).
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: channel.Of(r.Instrument), Name: r.Name, Snap: snap})
		}
	}
	st := SynthesizeAnalyzerState(channel.Master, "Master", snap, rowSnaps, nil, channel.ID{}, "", audio.SampleRate())
	testAnalyzerStateOverride = st

	// The same snapshot in every stage slot: the renderer only needs Active on the exercised pair.
	testScopeStateOverride = SynthesizeScopeState(channel.Master, scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Master: snap})

	t.Cleanup(func() {
		testAnalyzerStateOverride = nil
		testScopeStateOverride = nil
	})

	r, ok := g.SubjectRect(subject)
	if !ok || r.Empty() {
		t.Fatalf("SubjectRect(%q) ok=%v rect=%v", subject, ok, r)
	}

	screen := ebiten.NewImage(1280, 720)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})
	return r, rects
}

// Each tab draws data in its own colours.
func countDataRects(tab PanelTab, rects []drawnRect, inside image.Rectangle) int {
	switch tab {
	case TabWave:
		// drawWaveTrace emits one colWaveTrace rect per pixel column.
		return rectsWithColorInside(rects, inside, colWaveTrace)
	case TabSpectrum:
		// The bar-top band is drawn once per bar and never as chrome; mid/base count so a height-1 bar registers.
		return rectsWithColorInside(rects, inside, colSpectrumBarTop, colSpectrumBarMid, colSpectrumBarBase)
	case TabMeters:
		return barRectsInsideMeter(rects, inside)
	case TabScope:
		return rectsWithColorInside(rects, inside, colScopeA, colScopeB, colScopeDiff)
	}
	return 0
}

// Conservative floors: well above residue, robust to renderer detail (Wave draws ~one rect per column).
func TestAudioPanelTabsRenderNonBlankPixels(t *testing.T) {
	cases := []struct {
		name     string
		scene    string
		subject  Subject
		tab      PanelTab
		minRects int
	}{
		{"Wave", "crop_eq_tab_wave", SubjectEQTabWave, TabWave, 40},
		// ~10 bars plus peak markers, some at the floor.
		{"Spectrum", "crop_eq_tab_spectrum", SubjectEQTabSpectrum, TabSpectrum, 6},
		// A peak bar and an RMS overlay per row, plus the master pair.
		{"Meters", "crop_eq_tab_levels", SubjectEQTabLevels, TabMeters, 2},
		// Per-column rects per trace; 20 allows narrow scenes.
		{"Scope", "crop_chain_default", SubjectChain, TabScope, 20},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			g := driveScene(t, c.scene)
			rect, rects := drawWithSnapshot(t, g, c.subject, snapshotWithSine())
			n := countDataRects(c.tab, rects, rect)
			if n < c.minRects {
				t.Errorf("%s panel: only %d data-color rects in subject %v (want ≥ %d) — the data path is broken or the renderer never ran",
					c.name, n, rect, c.minRects)
			}
			if testing.Verbose() {
				t.Logf("%s: rect=%v dataRects=%d", c.name, rect, n)
			}
		})
	}
}

// With no data, no data-colour rect may appear, whatever chrome draws. Scope is skipped: its taps default to
// inactive, the same as no audio.
func TestAudioPanelTabsBlankWithoutAudio(t *testing.T) {
	cases := []struct {
		name    string
		scene   string
		subject Subject
		tab     PanelTab
	}{
		{"Wave", "crop_eq_tab_wave", SubjectEQTabWave, TabWave},
		{"Spectrum", "crop_eq_tab_spectrum", SubjectEQTabSpectrum, TabSpectrum},
		{"Meters", "crop_eq_tab_levels", SubjectEQTabLevels, TabMeters},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			g := driveScene(t, c.scene)
			rect, rects := drawWithSnapshot(t, g, c.subject, audio.AnalyzerSnapshot{})
			n := countDataRects(c.tab, rects, rect)
			if n > 0 {
				t.Errorf("%s with empty snapshot: %d data-color rects in subject %v (want 0) — chrome should be the only thing drawn",
					c.name, n, rect)
			}
		})
	}
}

// If a scene stops playing or settling, this fails first with a clearer error.
func TestSceneCropAudioPanelsCoverScreen(t *testing.T) {
	requiredCropScenes := []string{
		"crop_eq_tab_wave",
		"crop_eq_tab_spectrum",
		"crop_eq_tab_levels",
		"crop_chain_default",
	}
	for _, name := range requiredCropScenes {
		found := false
		for _, s := range sceneCatalog {
			if s.Name != name {
				continue
			}
			found = true
			if s.SettleFrames < 30 {
				t.Errorf("scene %q SettleFrames=%d (want ≥30 — analyzer ring buffers need time to fill)",
					name, s.SettleFrames)
			}
			if s.Subject == SubjectFullScreen {
				t.Errorf("scene %q has empty Subject — pixel-content tests can't crop", name)
			}
		}
		if !found {
			t.Errorf("scene %q not in sceneCatalog (audio-panel pixel test depends on it)", name)
		}
	}
}

// The stripes, separators, labels and mute rects are chrome and render on the first frame, before any spectrum
// exists; no snapshot is injected on purpose.
func TestEQTabStripesRenderWithoutAnalyzerData(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_eq")

	rect, ok := g.SubjectRect(SubjectEQTabEQ)
	if !ok || rect.Empty() {
		t.Fatalf("SubjectRect(eq) ok=%v rect=%v", ok, rect)
	}

	// A suspended browser AudioContext returns an empty snapshot until a gesture; the stub would hand back data and
	// mask it.
	empty := audio.AnalyzerSnapshot{}
	g.drum.eqTestSnapshot = &empty
	t.Cleanup(func() { g.drum.eqTestSnapshot = nil })

	// The stripes draw through a sprite in local coordinates; RENDER_SAFE sends every primitive through the
	// interceptor at screen coordinates.
	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })

	screen := ebiten.NewImage(1280, 720)
	rects := collectFilledRects(t, func() { g.Draw(screen) })

	// 10 bands give 10 fills; the floor stays well below that, and 0 means no backdrop.
	stripes := rectsWithColorInside(rects, rect,
		fadeColor(colEQBg, 0.2), fadeColor(colGridLine, 0.3))
	if stripes < 5 {
		t.Errorf("EQ tab striped background: only %d stripe rects in subject %v (want ≥ 5) — drawSpectrumBars dropped its static chrome when the analyzer had no data",
			stripes, rect)
	}
	if testing.Verbose() {
		t.Logf("EQ stripes (no analyzer data): rect=%v stripes=%d", rect, stripes)
	}
}
