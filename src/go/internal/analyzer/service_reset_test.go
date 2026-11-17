package analyzer

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestServiceResetClearsPublishedStateAndCapture covers the desktop side of the
// "audio tabs show garbage after loading a stopped circuit" bug. The analyzer
// service carries the last capture forward across ticks and keeps publishing it
// when no new audio flows (processTick: "carry forward last capture"). Loading a
// circuit must be able to flush that so the Wave/Spectrum/Levels tabs read flat
// until the new circuit is played.
func TestServiceResetClearsPublishedStateAndCapture(t *testing.T) {
	cfg := Config{FFTSize: 64, WindowSize: 128, MaxInstruments: 4, SampleRate: 44100}
	s := NewService(cfg)

	peakRMS := wave.NewPeakRMSObserver()
	masterFFT := wave.NewFFTObserver(cfg.FFTSize)
	envelope := wave.NewEnvelopeObserver(5, 50)
	masterBuf := make([]float64, cfg.WindowSize)
	instBuf := make([]float64, cfg.WindowSize)

	// Feed a full window of audio + a capture trigger, then run one tick so the
	// service publishes a non-flat state (this is the "circuit was playing"
	// state left behind at stop).
	loud := make([]float64, cfg.WindowSize)
	for i := range loud {
		loud[i] = 0.5
	}
	s.PushMasterBuf(loud)
	s.NotifyTrigger(channel.Of(roster.Kick), []float32{0.5, 0.5, 0.5, 0.5})

	st1 := s.processTick(masterBuf, instBuf, peakRMS, masterFFT, envelope)
	s.state.Store(st1)
	if !st1.Master.Active {
		t.Fatalf("precondition: expected active master after feeding audio")
	}
	if st1.Capture == nil {
		t.Fatalf("precondition: expected a capture after NotifyTrigger")
	}

	// Load-a-circuit flush.
	s.Reset()

	// The immediately-published state must be flat so the tabs stop showing the
	// previous circuit's frame the moment the load completes.
	got := s.State()
	if got == nil {
		t.Fatalf("expected a published state after Reset, got nil")
	}
	if got.Master.Active {
		t.Errorf("master still active after Reset (expected flat)")
	}
	if got.Capture != nil {
		t.Errorf("capture still present after Reset (expected nil)")
	}
	if len(got.Master.Waveform) != 0 {
		t.Errorf("master waveform not cleared after Reset: len=%d", len(got.Master.Waveform))
	}

	// A subsequent idle tick (no new audio, no trigger — playback is stopped)
	// must NOT resurrect the carried-forward capture.
	st2 := s.processTick(masterBuf, instBuf, peakRMS, masterFFT, envelope)
	if st2.Capture != nil {
		t.Errorf("carried-forward capture resurrected on idle tick after Reset")
	}
	if st2.Master.Active {
		t.Errorf("master active on idle tick after Reset (expected flat)")
	}
}
