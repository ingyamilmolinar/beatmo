package analyzer

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The service carries the last capture forward while no audio flows; a circuit load must flush it so the tabs read flat
// until the new circuit plays.
func TestServiceResetClearsPublishedStateAndCapture(t *testing.T) {
	cfg := Config{FFTSize: 64, WindowSize: 128, MaxInstruments: 4, SampleRate: 44100}
	s := NewService(cfg)

	peakRMS := wave.NewPeakRMSObserver()
	masterFFT := wave.NewFFTObserver(cfg.FFTSize)
	envelope := wave.NewEnvelopeObserver(5, 50)
	masterBuf := make([]float64, cfg.WindowSize)
	instBuf := make([]float64, cfg.WindowSize)

	// The state a playing circuit leaves behind at stop.
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
