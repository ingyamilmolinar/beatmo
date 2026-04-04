//go:build test

package scope

import (
	"math"
	"testing"
	"time"
)

func TestServicePublishesState(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 100,
		SampleRate:  44100,
	})

	svc.SetTapA(StageSynth)

	go svc.Run()
	defer svc.Stop()

	// Push a sine-like buffer to the synth stage.
	buf := make([]float64, 512)
	for i := range buf {
		buf[i] = math.Sin(2 * math.Pi * float64(i) / 64)
	}
	svc.PushSamples(StageSynth, "kick", buf)

	// Wait for the ticker to process.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.TapA.Active {
			if len(st.TapA.Samples) == 0 {
				t.Fatal("TapA active but no samples")
			}
			if st.TapA.Stage != StageSynth {
				t.Fatalf("expected stage StageSynth, got %v", st.TapA.Stage)
			}
			if st.TapA.InstID != "kick" {
				t.Fatalf("expected instID kick, got %q", st.TapA.InstID)
			}
			if st.TapA.PeakDB > 1 || st.TapA.PeakDB < -80 {
				t.Fatalf("unexpected peakDB: %f", st.TapA.PeakDB)
			}
			if st.TapA.RMSDB > 1 || st.TapA.RMSDB < -80 {
				t.Fatalf("unexpected rmsDB: %f", st.TapA.RMSDB)
			}
			// TapB should not be active.
			if st.TapB.Active {
				t.Fatal("TapB should not be active")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for state with active TapA")
}

func TestServiceFreeze(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 100,
		SampleRate:  44100,
	})

	svc.SetTapA(StageEQ)

	go svc.Run()
	defer svc.Stop()

	// Push initial samples.
	buf1 := make([]float64, 256)
	for i := range buf1 {
		buf1[i] = 0.5
	}
	svc.PushSamples(StageEQ, "snare", buf1)

	// Wait for state to be published with buf1.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.TapA.Active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := svc.State()
	if st == nil || !st.TapA.Active {
		t.Fatal("expected active TapA before freeze")
	}
	oldTS := st.Timestamp

	// Freeze and push different samples.
	svc.Freeze()

	if !svc.IsFrozen() {
		t.Fatal("expected frozen after Freeze()")
	}

	buf2 := make([]float64, 256)
	for i := range buf2 {
		buf2[i] = -0.9
	}
	svc.PushSamples(StageEQ, "snare", buf2)

	// Wait enough time for at least one tick to process.
	time.Sleep(100 * time.Millisecond)

	// State should still be the old one (timestamp unchanged).
	st2 := svc.State()
	if st2 == nil {
		t.Fatal("state is nil after freeze")
	}
	if st2.Timestamp != oldTS {
		t.Fatal("state was updated while frozen")
	}

	// Verify samples are from buf1, not buf2.
	for _, s := range st2.TapA.Samples {
		if s < 0 {
			t.Fatalf("found negative sample %f — state was overwritten while frozen", s)
		}
	}

	// Unfreeze and push new samples — state should update.
	svc.Unfreeze()
	buf3 := make([]float64, 256)
	for i := range buf3 {
		buf3[i] = 0.3
	}
	svc.PushSamples(StageEQ, "snare", buf3)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st3 := svc.State()
		if st3 != nil && st3.Timestamp != oldTS {
			return // State updated after unfreeze.
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for state update after unfreeze")
}

func TestServiceTapBoth(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 100,
		SampleRate:  44100,
	})

	svc.SetTapA(StageSynth)
	svc.SetTapB(StageMaster)

	go svc.Run()
	defer svc.Stop()

	buf := make([]float64, 256)
	for i := range buf {
		buf[i] = 0.7
	}
	svc.PushSamples(StageSynth, "kick", buf)
	svc.PushSamples(StageMaster, "kick", buf)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.TapA.Active && st.TapB.Active {
			if st.TapA.Stage != StageSynth {
				t.Fatalf("TapA stage: got %v, want StageSynth", st.TapA.Stage)
			}
			if st.TapB.Stage != StageMaster {
				t.Fatalf("TapB stage: got %v, want StageMaster", st.TapB.Stage)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for both taps active")
}

func TestServiceClearTaps(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 100,
		SampleRate:  44100,
	})

	svc.SetTapA(StageSynth)
	if svc.TapA() != StageSynth {
		t.Fatalf("expected StageSynth, got %v", svc.TapA())
	}

	svc.ClearTapA()
	if svc.TapA() != Stage(-1) {
		t.Fatalf("expected -1 after clear, got %v", svc.TapA())
	}

	svc.SetTapB(StageEQ)
	svc.ClearTapB()
	if svc.TapB() != Stage(-1) {
		t.Fatalf("expected -1 after clear, got %v", svc.TapB())
	}
}

func TestServiceInstrumentFilter(t *testing.T) {
	svc := NewService(Config{
		MaxWindowMs: 100,
		SampleRate:  44100,
	})

	svc.SetTapA(StageSynth)
	svc.SetInstrument("kick")

	go svc.Run()
	defer svc.Stop()

	// Push samples with a different instrument — should be ignored.
	buf := make([]float64, 256)
	for i := range buf {
		buf[i] = 0.5
	}
	svc.PushSamples(StageSynth, "snare", buf)

	time.Sleep(100 * time.Millisecond)
	st := svc.State()
	if st != nil && st.TapA.Active {
		t.Fatal("expected no active TapA for mismatched instrument")
	}

	// Push with matching instrument.
	svc.PushSamples(StageSynth, "kick", buf)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st = svc.State()
		if st != nil && st.TapA.Active {
			if st.TapA.InstID != "kick" {
				t.Fatalf("expected kick, got %q", st.TapA.InstID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for matching instrument state")
}

func TestBuildTapData(t *testing.T) {
	samples := []float64{0.0, 0.5, -1.0, 0.25}
	td := buildTapData(StageInsertFX, "hihat", samples)

	if !td.Active {
		t.Fatal("expected Active=true")
	}
	if td.Stage != StageInsertFX {
		t.Fatalf("expected StageInsertFX, got %v", td.Stage)
	}
	if td.InstID != "hihat" {
		t.Fatalf("expected hihat, got %q", td.InstID)
	}

	// Peak should be 1.0 (abs(-1.0)).
	expectedPeak := 20 * math.Log10(1.0) // 0 dB
	if math.Abs(td.PeakDB-expectedPeak) > 0.01 {
		t.Fatalf("expected peakDB ~%f, got %f", expectedPeak, td.PeakDB)
	}

	// RMS = sqrt((0 + 0.25 + 1.0 + 0.0625) / 4) = sqrt(0.328125)
	expectedRMS := 20 * math.Log10(math.Sqrt(0.328125))
	if math.Abs(td.RMSDB-expectedRMS) > 0.01 {
		t.Fatalf("expected rmsDB ~%f, got %f", expectedRMS, td.RMSDB)
	}
}

func TestRingBufDrain(t *testing.T) {
	r := &ringBuf{}

	// Drain empty buffer.
	samples, id := r.drain(100)
	if samples != nil {
		t.Fatal("expected nil from empty drain")
	}
	if id != "" {
		t.Fatalf("expected empty id, got %q", id)
	}

	// Push and drain with max limit.
	r.push("kick", []float64{1, 2, 3, 4, 5})
	samples, id = r.drain(3)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	if id != "kick" {
		t.Fatalf("expected kick, got %q", id)
	}
	// Should keep last 3: [3, 4, 5]
	if samples[0] != 3 || samples[1] != 4 || samples[2] != 5 {
		t.Fatalf("expected [3,4,5], got %v", samples)
	}

	// Second drain should be empty.
	samples, _ = r.drain(100)
	if samples != nil {
		t.Fatal("expected nil after drain")
	}
}
