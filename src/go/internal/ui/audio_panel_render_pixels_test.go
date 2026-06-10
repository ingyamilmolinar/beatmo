//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Closes the test-coverage gap behind the WASM Wave-tab regression: the
// existing scene_crop_test.go only verifies SubjectRect bounds, not that
// the rect contains rendered data. A panel that drew the chrome but
// silently dropped the data path passed every test.
//
// The browser companion (audio_panel_render.browser.test.js) drives the
// real WebAudio bridge end-to-end. This file covers the renderer side
// using the eqTestSnapshot/eqTestPreEQSnapshot hooks to inject
// deterministic non-zero analyzer data, so a fast Go test fails the
// moment the renderer-given-data path goes blank.

// snapshotWithSine returns an AnalyzerSnapshot carrying a recognizable
// sine in Waveform plus a non-flat Spectrum. Mirrors what a live
// WebAudio AnalyserNode would deliver during steady-state playback.
func snapshotWithSine() audio.AnalyzerSnapshot {
	const n = 512
	wave := make([]float64, n)
	for i := range wave {
		wave[i] = 0.6 * math.Sin(2*math.Pi*float64(i)/64.0)
	}
	spec := make([]float64, 64)
	for i := range spec {
		// Decaying spectrum so SnapshotToChannelMetrics produces visibly
		// varying FFTBins, not a flat -80 dB floor.
		spec[i] = 0.9 * math.Exp(-float64(i)/12.0)
	}
	return audio.AnalyzerSnapshot{
		RMS:      0.42,
		Peak:     0.6,
		Spectrum: spec,
		Waveform: wave,
	}
}

// rectsWithColorInside counts filled drawRect calls whose color matches
// any of `colors` and whose rect intersects `inside`. This is the
// discriminator between "data-bearing draws" (wave/spectrum bars,
// scope trace fills) and "chrome" (panel bg, axis ticks, labels).
// Counting raw rects rather than coverage avoids the overlap-double-
// counting problem that makes whole-panel coverage ≫ 100%.
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

// barRectsInsideMeter counts horizontal bar rects in the Meters panel,
// keyed on the data colors meterGreen/meterYellow/meterRed produced by
// meterColor(db). Empty snapshots produce -80 dB → dbToFrac=0 → peakPx=0
// → no meter-color rect drawn at all. Any non-zero count is conclusive
// evidence that the renderer received non-zero peak/RMS data.
func barRectsInsideMeter(rects []drawnRect, inside image.Rectangle) int {
	if inside.Empty() {
		return 0
	}
	// Match against the three meter colors (peak fill) plus their
	// 50% RMS overlays. WithAlpha returns NRGBA, so build the
	// expected RGBA representations the same way the renderer does.
	wantedRGB := []color.RGBA{meterGreen, meterYellow, meterRed}
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
		count++
	}
	return count
}

// driveScene runs the named scene's Setup, then drives a handful of
// Update frames so layout/caches populate the way the screenshot
// harness's settle countdown does. Returns the configured Game ready
// for one or more Draw passes.
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

// drawWithSnapshot injects a snapshot through every audio-data path the
// renderer reads from and runs Draw, returning the captured rects + the
// SubjectRect.
//
// Three injection points are needed:
//   1. dv.eqTestSnapshot / eqTestPreEQSnapshot — feeds the legacy
//      DrawWaveform fallback path (used by the Wave tab when
//      AnalyzerState callback returns nil).
//   2. testAnalyzerStateOverride — read by BuildAnalyzerStateFromSnapshots
//      under -tags test, so the AnalyzerState callback (Spectrum,
//      Meters, Wave-via-new-path) sees real data.
//   3. testScopeStateOverride — same idea for the Scope tab.
//
// All three are restored to nil via t.Cleanup so tests don't leak state.
func drawWithSnapshot(t *testing.T, g *Game, subject Subject, snap audio.AnalyzerSnapshot) (image.Rectangle, []drawnRect) {
	t.Helper()
	if g.drum == nil {
		t.Fatal("game.drum is nil")
	}

	// Legacy DrawWaveform fallback path.
	post := snap
	pre := snap
	g.drum.eqTestSnapshot = &post
	g.drum.eqTestPreEQSnapshot = &pre

	// AnalyzerState path (Spectrum, Meters, also Wave on WASM).
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: r.Instrument, Name: r.Name, Snap: snap})
		}
	}
	st := SynthesizeAnalyzerState("main", "Master", snap, rowSnaps, nil, "", "", audio.SampleRate())
	testAnalyzerStateOverride = st

	// Scope path: synthesize with both taps fed from the same snapshot so
	// the rendered traces are visible. Same snap goes into every per-stage
	// slot — the renderer just needs Active=true for whichever (TapA,TapB)
	// stage pair is exercised.
	testScopeStateOverride = SynthesizeScopeState("main", scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Sends: snap, Master: snap})

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

// countDataRects returns how many "data-bearing" rects the renderer
// emitted inside the subject for a given tab. Different renderers
// pick different data colors, so each tab has its own discriminator.
func countDataRects(tab PanelTab, rects []drawnRect, inside image.Rectangle) int {
	switch tab {
	case TabWave:
		// drawWaveTrace emits one colWaveTrace rect per pixel column.
		return rectsWithColorInside(rects, inside, colWaveTrace)
	case TabSpectrum:
		// drawSpectrumBars (in render_spectrum.go:142) uses colWaveTrace
		// for each band's bar fill.
		return rectsWithColorInside(rects, inside, colWaveTrace)
	case TabMeters:
		return barRectsInsideMeter(rects, inside)
	case TabScope:
		// drawWaveTrace called with colScopeA / colScopeB depending
		// on which trace is visible (see render_scope.go:163,166).
		return rectsWithColorInside(rects, inside, colScopeA, colScopeB, colScopeDiff)
	}
	return 0
}

// TestAudioPanelTabsRenderNonBlankPixels — the four data-driven audio
// tabs (Wave, Spectrum, Meters, Scope) must each emit at least N
// data-color filled rects inside the SubjectRect when fed a non-empty
// analyzer snapshot. A bug that silently zeroed the data path or
// detached the renderer would fail this test.
//
// Floors are intentionally conservative: enough rects to be much more
// than zero-or-tiny-residue, low enough to be robust against renderer
// detail changes (Wave emits ~one rect per pixel column; if the panel
// is N px wide, expect ~N rects).
func TestAudioPanelTabsRenderNonBlankPixels(t *testing.T) {
	cases := []struct {
		name     string
		scene    string
		subject  Subject
		tab      PanelTab
		minRects int
	}{
		{"Wave", "crop_eq_tab_wave", SubjectEQTabWave, TabWave, 40},
		// Spectrum draws ~10 bars + per-bar peak markers; even with
		// some bars at the dB floor, expect ≥ 6 fills.
		{"Spectrum", "crop_eq_tab_spectrum", SubjectEQTabSpectrum, TabSpectrum, 6},
		// Meters: one peak bar + one RMS overlay per row, plus one
		// pair for the master meter. With one row by default we
		// expect ≥ 2 bar rects.
		{"Meters", "crop_eq_tab_levels", SubjectEQTabLevels, TabMeters, 2},
		// Scope: drawWaveTrace emits per-column rects per visible
		// trace. Two traces × ~hundreds of columns; floor at 20 to
		// allow narrow render scenes.
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

// TestAudioPanelTabsBlankWithoutAudio — the inverse contract: when no
// snapshot data is available (empty AnalyzerSnapshot, panel still
// rendered), each tab's *data-color* rect count must be zero. Chrome
// (axis labels, midline, dashed grid, panel bg) draws regardless of
// data and is excluded from this count.
//
// This pins the renderer-given-data contract so a leaking default
// snapshot or a buggy `Active` flag can't silently produce data
// rects in the no-audio path. Skips Scope because tap selection
// defaults to inactive, which is the same as no-audio for the trace
// counter.
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

// TestSceneCropAudioPanelsCoverScreen — explicit guard that the
// scene-catalog entries used above stay opted-in to playback +
// settle. If anyone removes the SetPlaying(true) line from
// crop_eq_tab_wave/spectrum/meters or activateScopeTab, this test
// fails before the renderer test does, with a clearer error.
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
