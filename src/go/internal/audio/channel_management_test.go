package audio

import (
	"math"
	"sync"
	"testing"
)

func TestSetVolumeNaN(t *testing.T) {
	ch := newChannel("vol-nan", nil)
	ch.SetVolume(math.NaN())
	if v := ch.Volume(); v != 1.0 {
		t.Errorf("SetVolume(NaN) → Volume() = %f, want 1.0", v)
	}
}

func TestSetVolumeInf(t *testing.T) {
	ch := newChannel("vol-inf", nil)
	ch.SetVolume(math.Inf(1))
	if v := ch.Volume(); v != 1.0 {
		t.Errorf("SetVolume(+Inf) → Volume() = %f, want 1.0", v)
	}
}

func TestSetVolumeNegative(t *testing.T) {
	ch := newChannel("vol-neg", nil)
	ch.SetVolume(-0.5)
	if v := ch.Volume(); v != 0 {
		t.Errorf("SetVolume(-0.5) → Volume() = %f, want 0", v)
	}
}

func TestConcurrentVolumeAndProcessing(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("conc-vol", nil)

	var wg sync.WaitGroup
	// 4 goroutines changing volume.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ch.SetVolume(float64(i%100) / 100.0)
			}
		}()
	}
	// 1 goroutine processing.
	wg.Add(1)
	go func() {
		defer wg.Done()
		input := make([]float64, 64)
		output := make([]float64, 64)
		for i := range input {
			input[i] = 0.5
		}
		for i := 0; i < 100; i++ {
			for j := range output {
				output[j] = 0
			}
			ch.ProcessBlockLocal(input, output)
		}
	}()
	wg.Wait()
}

func TestConcurrentPanAndProcessing(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("conc-pan", nil)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ch.SetPan(float64(i%200-100) / 100.0)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = ch.PanGains()
		}
	}()
	wg.Wait()
}

func TestResetInstrumentChannels(t *testing.T) {
	withDefaultAudio(t)
	// Create some instrument channels.
	_ = InstrumentChannel("reset-a")
	_ = InstrumentChannel("reset-b")
	chanMgr.ensureChannel("reset-a").SetVolume(0.3)

	// Set a sentinel master volume so the preservation check is independent of
	// the fresh-session master default (0.5, see channelManager.reset).
	chanMgr.main.SetVolume(0.7)

	resetInstrumentChannels([]string{"reset-c"})

	// Main should be preserved (resetInstruments must not recreate/reset it).
	if chanMgr.main == nil {
		t.Fatal("main channel is nil after reset")
	}
	if chanMgr.main.Volume() != 0.7 {
		t.Errorf("main volume = %f after reset, want 0.7 (preserved)", chanMgr.main.Volume())
	}

	// Old instrument channels should be gone.
	chanMgr.mu.RLock()
	_, aExists := chanMgr.instruments["reset-a"]
	_, cExists := chanMgr.instruments["reset-c"]
	chanMgr.mu.RUnlock()
	if aExists {
		t.Error("old instrument 'reset-a' should be cleared after resetInstrumentChannels")
	}
	if !cExists {
		t.Error("new instrument 'reset-c' should exist after resetInstrumentChannels")
	}
}

func TestEnsureChannelIdempotent(t *testing.T) {
	withDefaultAudio(t)
	ch1 := chanMgr.ensureChannel("idempotent")
	ch2 := chanMgr.ensureChannel("idempotent")
	if ch1 != ch2 {
		t.Error("ensureChannel returned different pointers for same ID")
	}
}

func TestChannelParentHierarchy(t *testing.T) {
	withDefaultAudio(t)
	instCh := InstrumentChannel("hierarchy-test")
	if instCh.parent != chanMgr.main {
		t.Error("instrument channel parent should be main")
	}
	if chanMgr.main.parent != nil {
		t.Error("main channel parent should be nil")
	}
}
