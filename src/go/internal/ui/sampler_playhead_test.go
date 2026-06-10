//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// fEq compares two floats with a small tolerance.
func fEq(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

func ms(n int) int64 { return int64(n) * int64(time.Millisecond) }

// ── Pure position math: samplerPlayheadFrac ──────────────────────────────────

func TestSamplerPlayheadFrac_ForwardSweep(t *testing.T) {
	assertDefaultParityState(t)
	cases := []struct {
		name     string
		elapsed  float64
		wantFrac float64
	}{
		{"start", 0.0, 0.2},
		{"mid", 0.5, 0.5},
		{"end", 1.0, 0.8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frac, vis := samplerPlayheadFrac(c.elapsed, 1.0, 0.2, 0.8)
			if !vis {
				t.Fatalf("elapsed %v: want visible", c.elapsed)
			}
			if !fEq(frac, c.wantFrac) {
				t.Errorf("elapsed %v: frac = %v, want %v", c.elapsed, frac, c.wantFrac)
			}
		})
	}
}

// The playhead always sweeps forward (left→right), even for a reversed sound.
// Reversal flips the buffer in place (see sampler_reverse_test.go); the highlight
// has no direction flag, so it can never run right→left.
func TestSamplerPlayheadFrac_AlwaysSweepsForward(t *testing.T) {
	assertDefaultParityState(t)
	if frac, vis := samplerPlayheadFrac(0.0, 1.0, 0.2, 0.8); !vis || !fEq(frac, 0.2) {
		t.Errorf("start: frac=%v vis=%v, want 0.2 true (left edge of kept region)", frac, vis)
	}
	if frac, vis := samplerPlayheadFrac(1.0, 1.0, 0.2, 0.8); !vis || !fEq(frac, 0.8) {
		t.Errorf("end: frac=%v vis=%v, want 0.8 true (right edge of kept region)", frac, vis)
	}
}

func TestSamplerPlayheadFrac_InvisibleOutsideRange(t *testing.T) {
	assertDefaultParityState(t)
	if _, vis := samplerPlayheadFrac(-0.01, 1.0, 0.0, 1.0); vis {
		t.Error("negative elapsed should be invisible (not yet triggered)")
	}
	if _, vis := samplerPlayheadFrac(1.5, 1.0, 0.0, 1.0); vis {
		t.Error("elapsed past duration should be invisible (playback finished)")
	}
}

func TestSamplerPlayheadFrac_ZeroDurationInvisible(t *testing.T) {
	assertDefaultParityState(t)
	if _, vis := samplerPlayheadFrac(0.5, 0.0, 0.0, 1.0); vis {
		t.Error("zero duration should be invisible (nothing to play)")
	}
	if _, vis := samplerPlayheadFrac(0.5, -1.0, 0.0, 1.0); vis {
		t.Error("negative duration should be invisible")
	}
}

func TestSamplerPlayheadFrac_OrdersTrimDefensively(t *testing.T) {
	assertDefaultParityState(t)
	frac, vis := samplerPlayheadFrac(0.5, 1.0, 0.8, 0.2)
	if !vis || !fEq(frac, 0.5) {
		t.Errorf("unordered trim: frac=%v vis=%v, want 0.5 true", frac, vis)
	}
}

// ── Duration: audibleDurationSeconds ─────────────────────────────────────────

func TestSamplerAudibleDuration_FullBuffer(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{rawSampleRate: 48000, raw: make([]float32, 48000)}
	s.reset()
	if got := s.audibleDurationSeconds(); !fEq(got, 1.0) {
		t.Errorf("full 1s buffer: duration = %v, want 1.0", got)
	}
}

func TestSamplerAudibleDuration_Trimmed(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{rawSampleRate: 48000, raw: make([]float32, 48000)}
	s.reset()
	s.startFrac, s.endFrac = 0.25, 0.75
	if got := s.audibleDurationSeconds(); !fEq(got, 0.5) {
		t.Errorf("half-trimmed buffer: duration = %v, want 0.5", got)
	}
}

func TestSamplerAudibleDuration_PitchScales(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{rawSampleRate: 48000, raw: make([]float32, 48000)}
	s.reset()
	s.transposeSemis = 12
	if got := s.audibleDurationSeconds(); !fEq(got, 0.5) {
		t.Errorf("+12st: duration = %v, want 0.5", got)
	}
	s.transposeSemis = -12
	if got := s.audibleDurationSeconds(); !fEq(got, 2.0) {
		t.Errorf("-12st: duration = %v, want 2.0", got)
	}
}

func TestSamplerAudibleDuration_EmptyBuffer(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reset()
	if got := s.audibleDurationSeconds(); got != 0 {
		t.Errorf("empty buffer: duration = %v, want 0", got)
	}
}

// ── Tracker: spawning, overlap, anti-jitter, prune, cap ──────────────────────

func TestPlayheadTracker_SpawnsOneLinePerNewTrigger(t *testing.T) {
	assertDefaultParityState(t)
	var tr samplerPlayheadTracker
	base := int64(1e9)
	tr.observe(base, map[string]int64{"a": base}, 1.0)
	if len(tr.lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(tr.lines))
	}
	// Re-observing the SAME trigger (every frame the glue re-passes it) must not
	// duplicate the line.
	tr.observe(base+ms(100), map[string]int64{"a": base}, 1.0)
	if len(tr.lines) != 1 {
		t.Errorf("re-observe of same trigger spawned a duplicate: %d", len(tr.lines))
	}
}

func TestPlayheadTracker_OverlappingTriggersCoexistWithDistinctColors(t *testing.T) {
	assertDefaultParityState(t)
	// The headline feature: when a beat re-fires before the previous sound has
	// finished, both lines must coexist with different colors.
	var tr samplerPlayheadTracker
	base := int64(1e9)
	tr.observe(base, map[string]int64{"kick": base}, 1.0) // line A at t=0
	mid := base + ms(500)
	tr.observe(mid, map[string]int64{"kick": mid}, 1.0) // line B, A still playing
	if len(tr.lines) != 2 {
		t.Fatalf("want 2 concurrent lines, got %d", len(tr.lines))
	}
	if tr.lines[0].colorIdx == tr.lines[1].colorIdx {
		t.Error("concurrent playhead lines share a color — must be distinct")
	}
	if tr.lines[0].startNanos != base {
		t.Errorf("first line start moved to %d (it was reset!) — must stay %d", tr.lines[0].startNanos, base)
	}
}

func TestPlayheadTracker_SameIdRefireSpawnsNewLine(t *testing.T) {
	assertDefaultParityState(t)
	// "subdivision of the next beat shorter than the duration": the same node
	// re-fires mid-sound and gets its own line.
	var tr samplerPlayheadTracker
	base := int64(1e9)
	tr.observe(base, map[string]int64{"kick": base}, 2.0)
	tr.observe(base+ms(500), map[string]int64{"kick": base + ms(500)}, 2.0)
	if len(tr.lines) != 2 {
		t.Fatalf("a re-fire within the sound's duration must add a 2nd line, got %d", len(tr.lines))
	}
}

func TestPlayheadTracker_SingleLineAdvancesMonotonically(t *testing.T) {
	assertDefaultParityState(t)
	// Anti-jitter: a single line's fraction must increase smoothly with time and
	// never jump backward.
	var tr samplerPlayheadTracker
	base := int64(1e9)
	tr.observe(base, map[string]int64{"a": base}, 2.0)
	prev := -1.0
	for t0 := 0; t0 <= 2000; t0 += 25 {
		now := base + ms(t0)
		views := tr.active(now, 2.0, 0.0, 1.0)
		if len(views) != 1 {
			t.Fatalf("t=%dms: got %d views, want 1", t0, len(views))
		}
		if views[0].frac+1e-9 < prev {
			t.Fatalf("t=%dms: frac %v jumped backward from %v (jitter)", t0, views[0].frac, prev)
		}
		prev = views[0].frac
	}
}

func TestPlayheadTracker_RetriggerDoesNotResetInflightLine(t *testing.T) {
	assertDefaultParityState(t)
	// Reproduces the pre-fix jitter: when the loaded instrument re-fired during a
	// preview, the old min()-of-two-clocks design yanked the line back to 0. Now
	// the in-flight preview line keeps its position and the re-fire adds a 2nd.
	var tr samplerPlayheadTracker
	pv := int64(1e9)
	tr.observe(pv, map[string]int64{"preview": pv, "kick": 0}, 2.0)
	mid := pv + ms(500)
	tr.observe(mid, map[string]int64{"preview": pv, "kick": mid}, 2.0)
	views := tr.active(mid, 2.0, 0.0, 1.0)
	if len(views) != 2 {
		t.Fatalf("want preview + kick lines = 2, got %d", len(views))
	}
	foundQuarter := false // preview at 0.5s of 2.0s = 0.25
	for _, v := range views {
		if v.frac > 0.24 && v.frac < 0.26 {
			foundQuarter = true
		}
	}
	if !foundQuarter {
		t.Errorf("preview line was reset by the kick re-trigger: views=%v", views)
	}
}

func TestPlayheadTracker_PrunesFinishedLines(t *testing.T) {
	assertDefaultParityState(t)
	var tr samplerPlayheadTracker
	base := int64(1e9)
	tr.observe(base, map[string]int64{"a": base}, 1.0)
	tr.observe(base+ms(1100), map[string]int64{"a": base}, 1.0) // a finished (>1s)
	if len(tr.lines) != 0 {
		t.Errorf("finished line not pruned: %d", len(tr.lines))
	}
}

func TestPlayheadTracker_CapsConcurrentLines(t *testing.T) {
	assertDefaultParityState(t)
	var tr samplerPlayheadTracker
	base := int64(1e9)
	for i := 0; i < samplerMaxPlayheads+3; i++ {
		tn := base + ms(i)
		tr.observe(tn, map[string]int64{fmt.Sprintf("id%d", i): tn}, 10.0)
	}
	if len(tr.lines) != samplerMaxPlayheads {
		t.Errorf("got %d lines, want cap %d", len(tr.lines), samplerMaxPlayheads)
	}
}

func TestPlayheadTracker_IgnoresStaleTriggerOnFirstObserve(t *testing.T) {
	assertDefaultParityState(t)
	// Opening the panel long after a trigger must not spawn a phantom line.
	var tr samplerPlayheadTracker
	old := int64(1e9)
	now := old + int64(5*time.Second) // dur 1s → 5s ago is stale
	tr.observe(now, map[string]int64{"a": old}, 1.0)
	if len(tr.lines) != 0 {
		t.Errorf("spawned a line for a stale trigger: %d", len(tr.lines))
	}
}

func TestPlayheadTracker_NeverSentinelIgnored(t *testing.T) {
	assertDefaultParityState(t)
	var tr samplerPlayheadTracker
	tr.observe(int64(1e9), map[string]int64{"a": 0}, 1.0) // 0 == never triggered
	if len(tr.lines) != 0 {
		t.Errorf("a never-triggered id (0) must not spawn a line, got %d", len(tr.lines))
	}
}

func TestSamplerPlayheadColor_CyclesDistinctly(t *testing.T) {
	assertDefaultParityState(t)
	if sameRGBA(samplerPlayheadColor(0), samplerPlayheadColor(1)) {
		t.Error("adjacent playhead colors must differ")
	}
	if n := len(customPalette); n > 0 && !sameRGBA(samplerPlayheadColor(0), samplerPlayheadColor(n)) {
		t.Error("playhead color must cycle with the palette length")
	}
}

// ── Draw glue ────────────────────────────────────────────────────────────────

// loadTestSampler installs a deterministic 1-second buffer at 48 kHz.
func loadTestSampler(dv *DrumView) {
	s := &dv.sampler
	s.raw = make([]float32, 48000)
	s.rawSampleRate = 48000
	s.captureID = "kick"
	s.reset()
}

func firstIndexColor(calls *[]drawRectCall, want color.Color) int {
	for i, c := range *calls {
		if sameRGBA(c.col, want) {
			return i
		}
	}
	return -1
}

func lastIndexColor(calls *[]drawRectCall, want color.Color) int {
	idx := -1
	for i, c := range *calls {
		if sameRGBA(c.col, want) {
			idx = i
		}
	}
	return idx
}

func TestSamplerWaveform_DrawsAllActivePlayheads(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	loadTestSampler(dv)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 40)
	now := int64(2e9)
	dv.sampler.playheads.lines = []samplerPlayheadLine{
		{startNanos: now - ms(500), colorIdx: 0}, // 0.5 / 1.0
		{startNanos: now - ms(200), colorIdx: 1}, // 0.2 / 1.0
	}
	restore := SwapSamplerNowFnForTest(func() int64 { return now })
	defer restore()

	calls := installDrawRectRecorder(t)
	dv.drawSamplerWaveform(nil)

	if n := countByColor(calls, samplerPlayheadColor(0)); n != 1 {
		t.Errorf("playhead color0 drawn %d times, want 1", n)
	}
	if n := countByColor(calls, samplerPlayheadColor(1)); n != 1 {
		t.Errorf("playhead color1 drawn %d times, want 1", n)
	}
}

func TestSamplerWaveform_PlayheadStrokeThickFromDensity(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	loadTestSampler(dv)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 40)
	now := int64(2e9)
	dv.sampler.playheads.lines = []samplerPlayheadLine{{startNanos: now - ms(500), colorIdx: 0}}
	restore := SwapSamplerNowFnForTest(func() int64 { return now })
	defer restore()

	// Stroke is density-driven (not a hardcoded width) and clearly thicker than
	// the old 2 px so the coloured lines are easy to see.
	want := Profile().DensityValues().SamplerPlayheadStroke
	if want < 3 {
		t.Fatalf("playhead stroke token = %d, want >= 3 (thicker for visibility)", want)
	}
	calls := installDrawRectRecorder(t)
	dv.drawSamplerWaveform(nil)
	found := false
	for _, c := range *calls {
		if sameRGBA(c.col, samplerPlayheadColor(0)) {
			found = true
			if c.rect.Dx() != want {
				t.Errorf("playhead line width = %d, want %d (density stroke)", c.rect.Dx(), want)
			}
		}
	}
	if !found {
		t.Fatal("no playhead drawn")
	}
}

func TestSamplerWaveform_NoPlayheadsWhenIdle(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	loadTestSampler(dv)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 40)
	// No active lines.
	restore := SwapSamplerNowFnForTest(func() int64 { return int64(2e9) })
	defer restore()
	calls := installDrawRectRecorder(t)
	dv.drawSamplerWaveform(nil)
	for i := 0; i < samplerMaxPlayheads; i++ {
		if n := countByColor(calls, samplerPlayheadColor(i)); n != 0 {
			t.Errorf("idle: playhead color%d drawn %d times, want 0", i, n)
		}
	}
}

func TestSamplerWaveform_PlayheadsDrawnOnTopOfHandles(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	loadTestSampler(dv)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 40)
	dv.sampler.startFrac, dv.sampler.endFrac = 0.1, 0.9
	now := int64(2e9)
	dv.sampler.playheads.lines = []samplerPlayheadLine{{startNanos: now - ms(500), colorIdx: 0}}
	restore := SwapSamplerNowFnForTest(func() int64 { return now })
	defer restore()

	calls := installDrawRectRecorder(t)
	dv.drawSamplerWaveform(nil)

	lastHandle := lastIndexColor(calls, colAccent) // trace + handles use colAccent
	firstPlayhead := firstIndexColor(calls, samplerPlayheadColor(0))
	if lastHandle < 0 {
		t.Fatal("expected handle (colAccent) draws")
	}
	if firstPlayhead < 0 {
		t.Fatal("expected a playhead draw")
	}
	if firstPlayhead <= lastHandle {
		t.Errorf("playhead at %d must be drawn after the handles (last at %d) — on top", firstPlayhead, lastHandle)
	}
}

func TestSamplerWaveform_PlayheadResizeInvariant(t *testing.T) {
	assertDefaultParityState(t)
	now := int64(2e9)
	restore := SwapSamplerNowFnForTest(func() int64 { return now })
	defer restore()

	fracOf := func(r image.Rectangle) float64 {
		dv := &DrumView{}
		loadTestSampler(dv)
		dv.sampler.waveformRect = r
		dv.sampler.playheads.lines = []samplerPlayheadLine{{startNanos: now - ms(250), colorIdx: 0}}
		calls := installDrawRectRecorder(t)
		dv.drawSamplerWaveform(nil)
		for _, c := range *calls {
			if sameRGBA(c.col, samplerPlayheadColor(0)) {
				cx := (c.rect.Min.X + c.rect.Max.X) / 2 // line centre, any stroke width
				return float64(cx-r.Min.X) / float64(r.Dx())
			}
		}
		t.Fatalf("no playhead drawn for rect %v", r)
		return 0
	}
	narrow := fracOf(image.Rect(0, 0, 100, 20))
	wide := fracOf(image.Rect(50, 0, 850, 20))
	if narrow < 0.24 || narrow > 0.26 {
		t.Errorf("x-fraction = %v, want ~0.25", narrow)
	}
	if d := narrow - wide; d > 0.02 || d < -0.02 {
		t.Errorf("x-fraction differs across resize: narrow=%v wide=%v", narrow, wide)
	}
}

// ── Integration: real Preview spawns a line through the update glue ──────────

func TestSamplerPreview_SpawnsPlayheadLine(t *testing.T) {
	g := samplerLayoutGame(t)
	dv := g.drum
	loadTestSampler(dv)
	dv.sampler.captureID = dv.samplerActiveInstrument()

	contentR := dv.eqPanelZone.contentRect()
	dv.buildSamplerTab(contentR, dv.samplerActiveInstrument())
	if dv.sampler.waveformRect.Empty() {
		t.Fatal("waveformRect empty after layout")
	}

	audio.ResetTriggerTimestamps()
	dv.samplerPreview()         // stamps the preview trigger (stub records)
	dv.updateSamplerPlayheads() // observe picks up the new trigger

	if len(dv.sampler.playheads.lines) == 0 {
		t.Fatal("Preview did not spawn a playhead line")
	}

	calls := installDrawRectRecorder(t)
	dv.drawSamplerWaveform(nil)
	found := false
	for i := 0; i < samplerMaxPlayheads; i++ {
		if countByColor(calls, samplerPlayheadColor(i)) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("no playhead line drawn after Preview through the full pipeline")
	}
}

func TestSamplerPreview_TriggersAudition(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	loadTestSampler(dv)
	var played string
	prev := SwapSamplerAuditionFnForTest(func(id string) { played = id })
	defer SwapSamplerAuditionFnForTest(prev)
	dv.samplerPreview()
	if played != samplerPreviewID {
		t.Errorf("preview auditioned %q, want %q", played, samplerPreviewID)
	}
}
