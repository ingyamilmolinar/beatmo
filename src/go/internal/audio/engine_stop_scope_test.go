//go:build !test

package audio

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Every block stage (AntiPop, InsertFX, EQ, Master) receives samples through Read(). StageSynth is pushed at
// Schedule() from a cVoice, which testVoice is not, so it is covered elsewhere.
func TestAllStagePushesDuringRead(t *testing.T) {
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	// A fresh service, so the package pointer is non-nil and its state can be polled.
	prev := scopeSvc
	svc := scope.NewService(scope.Config{MaxWindowMs: 200, SampleRate: sampleRate})
	scopeSvc = svc
	t.Cleanup(func() {
		svc.Stop()
		scopeSvc = prev
	})
	go svc.Run()

	// No real oto context is needed.
	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}

	// Schedule a few overlapping voices to keep the mixer busy through Read().
	const voiceSamples = 8192
	for i := 0; i < 3; i++ {
		m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)
	}

	readMixerSamples(m, voiceSamples)

	blockStages := []scope.Stage{
		scope.StageAntiPop,
		scope.StageInsertFX,
		scope.StageEQ,
		scope.StageMaster,
	}
	for _, stage := range blockStages {
		stage := stage
		t.Run(scope.StageLabel(stage), func(t *testing.T) {
			svc.SetTapA(stage)
			defer svc.ClearTapA()

			// Push more audio so the new tap selection gets fresh data.
			for i := 0; i < 4; i++ {
				m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)
			}
			readMixerSamples(m, voiceSamples)

			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				st := svc.State()
				if st != nil && st.TapA.Active && st.TapA.Stage == stage && len(st.TapA.Samples) > 0 {
					return // success
				}
				// Kept producing so the Master stage always has fresh data.
				readMixerSamples(m, blockSize)
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("scope service never published an active TapA for stage %s — "+
				"mixer is not pushing %s samples (Chain tab would render blank)",
				scope.StageLabel(stage), scope.StageLabel(stage))
		})
	}
}
