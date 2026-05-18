//go:build !test

package audio

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestAllStagePushesDuringRead drives the desktop mixer through Read() and
// asserts that every block-pipeline stage — AntiPop, InsertFX, EQ, Sends,
// Master — receives samples in the scope service's ring buffers.
//
// StageSynth is covered separately by scope/service_test.go's
// TestServicePushAllStages because the mixer pushes Synth at Schedule() time
// from the raw cVoice buffer (engine_mixer.go); the test-only `testVoice`
// type here doesn't wrap a cVoice and therefore can't exercise that path
// without bringing the C synth into the test. The mixer-Read contract for
// the five block stages is the new piece this test pins.
func TestAllStagePushesDuringRead(t *testing.T) {
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	// Install a fresh scope service so the package-level pointer is non-nil
	// and so we can poll its state for stage activation.
	prev := scopeSvc
	svc := scope.NewService(scope.Config{MaxWindowMs: 200, SampleRate: sampleRate})
	scopeSvc = svc
	t.Cleanup(func() {
		svc.Stop()
		scopeSvc = prev
	})
	go svc.Run()

	// Tap each pair (A, B) we need to verify; we poll the service per-stage.
	// Using the same mixer pattern as mixer_signal_test.go so we don't need
	// a real oto context.
	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[string]int),
	}

	// Schedule a few overlapping voices to keep the mixer busy through Read().
	const voiceSamples = 8192
	for i := 0; i < 3; i++ {
		m.Schedule("kick", newSineVoice(440, sampleRate, voiceSamples), 0)
	}

	// Drive enough samples through Read() so every block-processing stage
	// gets at least one push.
	readMixerSamples(m, voiceSamples)

	// For each block-pipeline stage, set it as TapA and wait for the service
	// to publish an active TapData. Synth is tested separately (see header).
	blockStages := []scope.Stage{
		scope.StageAntiPop,
		scope.StageInsertFX,
		scope.StageEQ,
		scope.StageSends,
		scope.StageMaster,
	}
	for _, stage := range blockStages {
		stage := stage
		t.Run(scope.StageLabel(stage), func(t *testing.T) {
			svc.SetTapA(stage)
			defer svc.ClearTapA()

			// Push more audio so the new tap selection gets fresh data.
			for i := 0; i < 4; i++ {
				m.Schedule("kick", newSineVoice(440, sampleRate, voiceSamples), 0)
			}
			readMixerSamples(m, voiceSamples)

			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				st := svc.State()
				if st != nil && st.TapA.Active && st.TapA.Stage == stage && len(st.TapA.Samples) > 0 {
					return // success
				}
				// Keep the mixer producing audio while we wait so master-path
				// stages (Sends/Master) always have fresh data to publish.
				readMixerSamples(m, blockSize)
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("scope service never published an active TapA for stage %s — "+
				"mixer is not pushing %s samples (Chain tab would render blank)",
				scope.StageLabel(stage), scope.StageLabel(stage))
		})
	}
}
