package analyzer

import (
	"math"
	"testing"
	"time"
)

// sineSamples generates n samples of a sine wave at the given frequency and sample rate.
func sineSamples(n int, freq float64, sampleRate int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.8 * math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate))
	}
	return out
}

// pollState polls s.State() until pred returns true or deadline expires.
func pollState(s *Service, deadline time.Duration, pred func(*State) bool) *State {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		st := s.State()
		if st != nil && pred(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s.State()
}

func TestServicePublishesState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SampleRate = 44100
	cfg.WindowSize = 2048
	cfg.FFTSize = 1024

	svc := NewService(cfg)
	go svc.Run()
	defer svc.Stop()

	// Push a sine wave into the master ring buffer.
	samples := sineSamples(cfg.WindowSize, 440, cfg.SampleRate)
	svc.PushMasterBuf(samples)

	// Wait for state to appear with a reasonable PeakDB.
	st := pollState(svc, 1*time.Second, func(s *State) bool {
		return s.Master.PeakDB > -100
	})

	if st == nil {
		t.Fatal("state never published")
	}
	// 0.8 amplitude => ~-1.94 dB. Allow some margin.
	if st.Master.PeakDB < -10 || st.Master.PeakDB > 0 {
		t.Errorf("Master.PeakDB = %f, want roughly -2 dB", st.Master.PeakDB)
	}
	if st.Master.RMSDB < -20 || st.Master.RMSDB > 0 {
		t.Errorf("Master.RMSDB = %f, want reasonable value", st.Master.RMSDB)
	}
	if len(st.Master.FFTBins) == 0 {
		t.Error("expected master FFTBins to be populated")
	}
	if len(st.Master.Envelope) == 0 {
		t.Error("expected master Envelope to be populated")
	}
	if st.Timestamp <= 0 {
		t.Error("expected positive Timestamp")
	}
}

func TestServiceInstrumentMetrics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SampleRate = 44100
	cfg.WindowSize = 2048

	svc := NewService(cfg)
	svc.RegisterInstrument(0, "kick", "Kick")
	go svc.Run()
	defer svc.Stop()

	samples := sineSamples(cfg.WindowSize, 100, cfg.SampleRate)
	svc.PushInstBuf(0, samples)

	st := pollState(svc, 1*time.Second, func(s *State) bool {
		for _, inst := range s.Instruments {
			if inst.ID == "kick" && inst.PeakDB > -100 {
				return true
			}
		}
		return false
	})

	if st == nil {
		t.Fatal("state never published with instrument metrics")
	}

	found := false
	for _, inst := range st.Instruments {
		if inst.ID == "kick" {
			found = true
			if inst.Name != "Kick" {
				t.Errorf("instrument Name = %q, want %q", inst.Name, "Kick")
			}
			if inst.PeakDB < -10 || inst.PeakDB > 0 {
				t.Errorf("inst.PeakDB = %f, want roughly -2 dB", inst.PeakDB)
			}
			if !inst.Active {
				t.Error("expected instrument to be Active")
			}
		}
	}
	if !found {
		t.Error("instrument 'kick' not found in state")
	}
}

// TestPushInstBufOutOfRangeSlot verifies that pushing into a slot index at or
// beyond MaxInstruments is silently dropped rather than panicking. The mixer
// hands out an ever-increasing slot index per unique instrument ID it has ever
// seen (engine_mixer.go instrumentSlot), with no cap. Once a session cycles a
// row through more than MaxInstruments (32) distinct instruments — easy to do
// by repeatedly changing a row's instrument, especially with clones — the mixer
// pushes slot >= 32 into the analyzer. RegisterInstrument already bounds-checks
// and silently drops such slots, but PushInstBuf did not, so s.slots[32] panicked
// with "index out of range [32] with length 32" on the audio thread.
func TestPushInstBufOutOfRangeSlot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxInstruments = 32

	svc := NewService(cfg)

	// Both of these must be no-ops (matching RegisterInstrument's contract),
	// not panics: a slot exactly at the length, and one well beyond it.
	svc.PushInstBuf(cfg.MaxInstruments, sineSamples(64, 100, cfg.SampleRate))
	svc.PushInstBuf(cfg.MaxInstruments+10, sineSamples(64, 100, cfg.SampleRate))
	svc.PushInstBuf(-1, sineSamples(64, 100, cfg.SampleRate))
}

func TestServiceCaptureTrigger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SampleRate = 44100
	cfg.WindowSize = 2048

	svc := NewService(cfg)
	go svc.Run()
	defer svc.Stop()

	// Push master data so the service produces a state.
	svc.PushMasterBuf(sineSamples(cfg.WindowSize, 440, cfg.SampleRate))

	// Trigger capture with a known buffer.
	capBuf := make([]float32, 512)
	for i := range capBuf {
		capBuf[i] = 0.5
	}
	svc.NotifyTrigger("snare", capBuf)

	st := pollState(svc, 1*time.Second, func(s *State) bool {
		return s.Capture != nil && s.Capture.InstID == "snare"
	})

	if st == nil || st.Capture == nil {
		t.Fatal("capture never appeared in state")
	}
	if st.Capture.InstID != "snare" {
		t.Errorf("Capture.InstID = %q, want %q", st.Capture.InstID, "snare")
	}
	if len(st.Capture.Wave.Samples) != 512 {
		t.Errorf("Capture wave has %d samples, want 512", len(st.Capture.Wave.Samples))
	}
	// Verify samples were converted from float32 to float64.
	for i, s := range st.Capture.Wave.Samples {
		if math.Abs(s-0.5) > 1e-6 {
			t.Errorf("Capture.Wave.Samples[%d] = %f, want 0.5", i, s)
			break
		}
	}
}

func TestServiceFreeze(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SampleRate = 44100
	cfg.WindowSize = 2048

	svc := NewService(cfg)
	go svc.Run()
	defer svc.Stop()

	// Push master data.
	svc.PushMasterBuf(sineSamples(cfg.WindowSize, 440, cfg.SampleRate))

	// Trigger first capture.
	svc.NotifyTrigger("first", make([]float32, 64))

	st := pollState(svc, 1*time.Second, func(s *State) bool {
		return s.Capture != nil && s.Capture.InstID == "first"
	})
	if st == nil || st.Capture == nil {
		t.Fatal("first capture never appeared")
	}

	// Freeze — subsequent triggers should not replace the capture.
	svc.Freeze()

	// Trigger second capture.
	svc.NotifyTrigger("second", make([]float32, 64))

	// Push more data so the ticker processes.
	svc.PushMasterBuf(sineSamples(cfg.WindowSize, 440, cfg.SampleRate))

	// Wait a bit and verify capture still has first ID.
	time.Sleep(150 * time.Millisecond)

	st = svc.State()
	if st == nil || st.Capture == nil {
		t.Fatal("state or capture is nil after freeze")
	}
	if st.Capture.InstID != "first" {
		t.Errorf("Capture.InstID = %q after freeze, want %q", st.Capture.InstID, "first")
	}

	// Unfreeze — now second trigger (or a new one) should be picked up.
	svc.Unfreeze()
	svc.NotifyTrigger("third", make([]float32, 64))
	svc.PushMasterBuf(sineSamples(cfg.WindowSize, 440, cfg.SampleRate))

	st = pollState(svc, 1*time.Second, func(s *State) bool {
		return s.Capture != nil && s.Capture.InstID == "third"
	})
	if st == nil || st.Capture == nil {
		t.Fatal("capture never updated after unfreeze")
	}
	if st.Capture.InstID != "third" {
		t.Errorf("Capture.InstID = %q after unfreeze, want %q", st.Capture.InstID, "third")
	}
}

func TestServiceDetailChannel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SampleRate = 44100
	cfg.WindowSize = 2048
	cfg.FFTSize = 1024

	svc := NewService(cfg)
	svc.RegisterInstrument(0, "hihat", "HiHat")
	svc.SetDetailChannel("hihat")
	go svc.Run()
	defer svc.Stop()

	samples := sineSamples(cfg.WindowSize, 5000, cfg.SampleRate)
	svc.PushInstBuf(0, samples)
	// Also push master data so the service ticks produce full state.
	svc.PushMasterBuf(sineSamples(cfg.WindowSize, 440, cfg.SampleRate))

	st := pollState(svc, 1*time.Second, func(s *State) bool {
		return s.Detail != nil && len(s.Detail.FFTBins) > 0
	})

	if st == nil || st.Detail == nil {
		t.Fatal("detail channel never appeared in state")
	}
	if st.Detail.ID != "hihat" {
		t.Errorf("Detail.ID = %q, want %q", st.Detail.ID, "hihat")
	}
	if len(st.Detail.FFTBins) == 0 {
		t.Error("expected Detail.FFTBins to be populated")
	}
	if len(st.Detail.FreqBins) == 0 {
		t.Error("expected Detail.FreqBins to be populated")
	}
	if len(st.Detail.Envelope) == 0 {
		t.Error("expected Detail.Envelope to be populated")
	}
}
