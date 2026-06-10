//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func approx(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %v, want %v (±%v)", msg, got, want, tol)
	}
}

func TestLinearToDBSafe_Floor(t *testing.T) {
	approx(t, LinearToDBSafe(1.0), 0.0, 0.01, "unit amplitude -> 0 dB")
	approx(t, LinearToDBSafe(0.1), -20.0, 0.01, "0.1 linear -> -20 dB")
	approx(t, LinearToDBSafe(0.0), -80.0, 0.01, "zero -> -80 dB floor")
	approx(t, LinearToDBSafe(-0.5), -80.0, 0.01, "negative -> -80 dB floor")
}

func TestSpectrumLinearToDBBins_FreqLabels(t *testing.T) {
	// 4 bins at sr=8000 => Nyquist 4000, step = 4000/4 = 1000
	spec := []float64{1.0, 0.5, 0.25, 0.1}
	fft, freq := SpectrumLinearToDBBins(spec, 8000)
	if len(fft) != 4 || len(freq) != 4 {
		t.Fatalf("expected 4 bins, got fft=%d freq=%d", len(fft), len(freq))
	}
	approx(t, freq[0], 0, 0.01, "freq[0]")
	approx(t, freq[1], 1000, 0.01, "freq[1]")
	approx(t, freq[2], 2000, 0.01, "freq[2]")
	approx(t, freq[3], 3000, 0.01, "freq[3]")
	approx(t, fft[0], 0.0, 0.01, "fft[0] linear 1.0 -> 0 dB")
	approx(t, fft[1], -6.02, 0.1, "fft[1] linear 0.5 -> -6 dB")
}

func TestSpectrumLinearToDBBins_EmptyIn(t *testing.T) {
	fft, freq := SpectrumLinearToDBBins(nil, 44100)
	if fft != nil || freq != nil {
		t.Fatalf("empty input should produce nil/nil output, got fft=%v freq=%v", fft, freq)
	}
}

func TestSnapshotToChannelMetrics_Active(t *testing.T) {
	snap := audio.AnalyzerSnapshot{
		RMS:      0.3,
		Peak:     0.7,
		Spectrum: []float64{1.0, 0.5, 0.25, 0.1},
		Waveform: []float64{0.1, -0.1, 0.2, -0.2},
	}
	m := SnapshotToChannelMetrics("kick", "Kick", snap, 8000)
	if !m.Active {
		t.Fatal("expected Active=true when peak>0")
	}
	if m.ID != "kick" || m.Name != "Kick" {
		t.Fatalf("id/name mismatch: %q/%q", m.ID, m.Name)
	}
	if len(m.Waveform) != 4 {
		t.Fatalf("waveform not copied: len=%d", len(m.Waveform))
	}
	if len(m.FFTBins) != 4 || len(m.FreqBins) != 4 {
		t.Fatalf("fft bins not built: fft=%d freq=%d", len(m.FFTBins), len(m.FreqBins))
	}
	// Peak and RMS should be converted from linear to dB.
	approx(t, m.PeakDB, 20*math.Log10(0.7), 0.1, "PeakDB")
	approx(t, m.RMSDB, 20*math.Log10(0.3), 0.1, "RMSDB")
}

func TestSnapshotToChannelMetrics_InactiveOnZero(t *testing.T) {
	snap := audio.AnalyzerSnapshot{
		RMS: 0, Peak: 0,
		Spectrum: []float64{0, 0, 0, 0},
		Waveform: []float64{0, 0, 0, 0},
	}
	m := SnapshotToChannelMetrics("main", "Master", snap, 8000)
	if m.Active {
		t.Fatal("expected Active=false when everything is zero")
	}
}

func TestSnapshotToInstrumentMetrics_Fields(t *testing.T) {
	snap := audio.AnalyzerSnapshot{RMS: 0.5, Peak: 0.9}
	im := SnapshotToInstrumentMetrics("snare", "Snare", snap)
	if im.ID != "snare" || im.Name != "Snare" {
		t.Fatalf("id/name mismatch: %q/%q", im.ID, im.Name)
	}
	if !im.Active {
		t.Fatal("expected Active=true when peak>0")
	}
	approx(t, im.PeakDB, 20*math.Log10(0.9), 0.1, "PeakDB")
	approx(t, im.RMSDB, 20*math.Log10(0.5), 0.1, "RMSDB")
}

func TestSynthesizeAnalyzerState_InstrumentRows(t *testing.T) {
	master := audio.AnalyzerSnapshot{RMS: 0.2, Peak: 0.6, Spectrum: []float64{1.0, 0.5}, Waveform: []float64{0, 0.5}}
	rows := []RowSnapshot{
		{ID: "kick", Name: "Kick", Snap: audio.AnalyzerSnapshot{Peak: 0.4, RMS: 0.2}},
		{ID: "snare", Name: "Snare", Snap: audio.AnalyzerSnapshot{Peak: 0.3, RMS: 0.1}},
	}
	st := SynthesizeAnalyzerState("main", "Master", master, rows, nil, "", "", 8000)
	if st == nil {
		t.Fatal("expected non-nil state")
	}
	if st.Master.ID != "main" || st.Master.Name != "Master" {
		t.Fatalf("master id/name wrong: %q/%q", st.Master.ID, st.Master.Name)
	}
	if len(st.Instruments) != 2 {
		t.Fatalf("expected 2 instruments, got %d", len(st.Instruments))
	}
	if st.Instruments[0].ID != "kick" || st.Instruments[1].ID != "snare" {
		t.Fatalf("instrument IDs mismatch: %+v", st.Instruments)
	}
	if st.Detail != nil {
		t.Fatal("expected Detail=nil when no detail snapshot passed")
	}
}

func TestSynthesizeAnalyzerState_DetailPopulated(t *testing.T) {
	master := audio.AnalyzerSnapshot{}
	detail := audio.AnalyzerSnapshot{Peak: 0.8, RMS: 0.4, Spectrum: []float64{0.9, 0.2}}
	st := SynthesizeAnalyzerState("main", "Master", master, nil, &detail, "kick", "Kick", 8000)
	if st.Detail == nil {
		t.Fatal("expected Detail to be populated when detail arg is non-nil")
	}
	if st.Detail.ID != "kick" || st.Detail.Name != "Kick" {
		t.Fatalf("detail id/name wrong: %q/%q", st.Detail.ID, st.Detail.Name)
	}
	if !st.Detail.Active {
		t.Fatal("detail should be Active when peak>0")
	}
}

func TestSynthesizeScopeState_StageMapping(t *testing.T) {
	synth := audio.AnalyzerSnapshot{Peak: 0.5, RMS: 0.2, Waveform: []float64{0.1, 0.2, 0.3}}
	post := audio.AnalyzerSnapshot{Peak: 0.4, RMS: 0.15, Waveform: []float64{0.05, 0.1, 0.15}}
	snaps := ScopeStageSnapshots{Synth: synth, PostEQ: post}
	st := SynthesizeScopeState("kick", scope.StageSynth, scope.StageEQ, snaps)
	if st == nil {
		t.Fatal("expected non-nil scope state")
	}
	if st.TapA.Stage != scope.StageSynth || st.TapB.Stage != scope.StageEQ {
		t.Fatalf("stages wrong: A=%v B=%v", st.TapA.Stage, st.TapB.Stage)
	}
	if !st.TapA.Active || !st.TapB.Active {
		t.Fatal("both taps should be Active")
	}
	if len(st.TapA.Samples) != 3 || len(st.TapB.Samples) != 3 {
		t.Fatalf("samples not forwarded: A=%d B=%d", len(st.TapA.Samples), len(st.TapB.Samples))
	}
	if st.TapA.InstID != "kick" || st.TapB.InstID != "kick" {
		t.Fatalf("inst id mismatch: A=%q B=%q", st.TapA.InstID, st.TapB.InstID)
	}
}

// TestSynthesizeScopeState_AllSixStagesBridged pins the new contract: every
// scope.Stage has a real source in ScopeStageSnapshots. AntiPop deliberately
// shares the Synth snapshot (per the WASM AntiPop design decision); the
// other five stages each have their own dedicated source.
func TestSynthesizeScopeState_AllSixStagesBridged(t *testing.T) {
	mkSnap := func(tag string) audio.AnalyzerSnapshot {
		// Make each snapshot trivially distinguishable by its waveform contents.
		return audio.AnalyzerSnapshot{
			Peak:     0.5,
			Waveform: []float64{float64(len(tag)), 1.0, -1.0},
		}
	}
	snaps := ScopeStageSnapshots{
		Synth:  mkSnap("synth"),
		PreEQ:  mkSnap("preeq"),
		PostEQ: mkSnap("posteq"),
		Sends:  mkSnap("sends"),
		Master: mkSnap("master"),
	}

	cases := []struct {
		stage        scope.Stage
		expectPeakAt float64 // first sample value identifies which snapshot was selected
	}{
		{scope.StageSynth, float64(len("synth"))},
		{scope.StageAntiPop, float64(len("synth"))}, // shares Synth
		{scope.StageInsertFX, float64(len("preeq"))},
		{scope.StageEQ, float64(len("posteq"))},
		{scope.StageSends, float64(len("sends"))},
		{scope.StageMaster, float64(len("master"))},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(scope.StageLabel(tc.stage), func(t *testing.T) {
			st := SynthesizeScopeState("kick", tc.stage, tc.stage, snaps)
			if !st.TapA.Active {
				t.Fatalf("stage %s: TapA should be Active when its snapshot has non-empty waveform", scope.StageLabel(tc.stage))
			}
			if len(st.TapA.Samples) != 3 {
				t.Fatalf("stage %s: expected 3 samples, got %d", scope.StageLabel(tc.stage), len(st.TapA.Samples))
			}
			if st.TapA.Samples[0] != tc.expectPeakAt {
				t.Fatalf("stage %s: routed to wrong snapshot (samples[0]=%v want %v)",
					scope.StageLabel(tc.stage), st.TapA.Samples[0], tc.expectPeakAt)
			}
		})
	}
}

func TestSynthesizeScopeState_InactiveOnEmpty(t *testing.T) {
	st := SynthesizeScopeState("kick", scope.StageSynth, scope.StageEQ, ScopeStageSnapshots{})
	if st.TapA.Active || st.TapB.Active {
		t.Fatalf("expected inactive taps on empty snapshots: A=%v B=%v", st.TapA.Active, st.TapB.Active)
	}
}

// TestSynthesizeScopeState_PerInstrumentSplitActive pins the contract the
// original Chn-tab bug violated: when the WASM bridge returns a non-empty
// Synth snapshot AND a non-empty PreEQ snapshot for the same per-instrument
// id, both TapA (Synth) and TapB (InsertFX) must surface as Active so the
// renderer draws both traces. The bug was that EnableSynthAnalyzer was
// never called for per-instrument ids, so Synth came back empty; this test
// is the floor the WASM bridge has to clear once the analyser is wired.
func TestSynthesizeScopeState_PerInstrumentSplitActive(t *testing.T) {
	synth := audio.AnalyzerSnapshot{
		Peak: 0.6, RMS: 0.42,
		Waveform: []float64{0.1, 0.2, 0.3, 0.4},
	}
	preEQ := audio.AnalyzerSnapshot{
		Peak: 0.3, RMS: 0.21,
		Waveform: []float64{0.05, 0.1, 0.15, 0.2},
	}
	st := SynthesizeScopeState("kick", scope.StageSynth, scope.StageInsertFX, ScopeStageSnapshots{
		Synth: synth,
		PreEQ: preEQ,
	})
	if !st.TapA.Active {
		t.Fatalf("TapA(Synth) should be Active with non-empty Synth snapshot: %+v", st.TapA)
	}
	if !st.TapB.Active {
		t.Fatalf("TapB(InsertFX) should be Active with non-empty PreEQ snapshot: %+v", st.TapB)
	}
	if st.TapA.InstID != "kick" || st.TapB.InstID != "kick" {
		t.Fatalf("InstID lost in synthesise: A=%q B=%q", st.TapA.InstID, st.TapB.InstID)
	}
	// Pin the bug-shaped failure mode too: with an empty Synth slot the
	// renderer drops trace A (active=false) and the Chn legend loses its
	// "A:" line — exactly the screenshot symptom.
	stBroken := SynthesizeScopeState("kick", scope.StageSynth, scope.StageInsertFX, ScopeStageSnapshots{
		Synth: audio.AnalyzerSnapshot{},
		PreEQ: preEQ,
	})
	if stBroken.TapA.Active {
		t.Fatal("TapA should be inactive when the Synth snapshot is empty (simulates un-wired analyser)")
	}
	if !stBroken.TapB.Active {
		t.Fatal("TapB should still be active when only Synth is empty — drop is isolated to A")
	}
}
