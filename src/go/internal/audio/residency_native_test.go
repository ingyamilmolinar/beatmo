//go:build !test && !js

package audio

import (
	"math"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// And never cuts a tail.

func residencyReset(t *testing.T, held ...instrument.ID) {
	t.Helper()
	// The live mixer runs the same channels from the device callback; pause it so only this test's mixer touches them.
	if mix != nil && mix.player != nil {
		mix.player.Pause()
		t.Cleanup(mix.player.Play)
		// A read in flight still finishes; wait until the live clock stands still.
		for last := -1; ; {
			mix.mu.Lock()
			pos := mix.pos
			mix.mu.Unlock()
			if pos == last {
				break
			}
			last = pos
			time.Sleep(30 * time.Millisecond)
		}
	}
	stageReset(t)
	SetHeldInstruments(held)
	t.Cleanup(ResetHeldInstruments)
}

// Applies residency, as the live mixer does.
func renderResidency(notes []StageNote, total int) ([]float64, *mixer) {
	ResetDSPState()
	m := newTestMixer()
	m.residency = true
	for _, n := range notes {
		v := playedVoiceOrigin(n.Inst, n.Pitch, n.Dur, sampleRate, originPrewarm)
		m.Schedule(channel.Of(n.Inst), &scaledVoice{v: v, gain: n.Vol}, n.At, -1, -1)
	}
	return renderMixer(m, total), m
}

func lastAbove(x []float64, amp float64) int {
	for i := len(x) - 1; i >= 0; i-- {
		if math.Abs(x[i]) > amp {
			return i
		}
	}
	return -1
}

func longDelay() []EffectSlot {
	return []EffectSlot{{Type: EffectDelay, Enabled: true, Params: map[string]float64{"time": 1000, "feedback": 0.7, "mix": 1}}}
}

func TestReleasedChannelRingsOutDelayTail(t *testing.T) {
	residencyReset(t) // nothing held: the row was switched away
	id := roster.Snare
	SetInsertEffects(id, longDelay())
	sr := SampleRate()
	notes := []StageNote{{Inst: id, At: 100, Vol: 1, Dur: 1}}
	total := 40 * sr

	ref, _ := RenderStagesOffline(notes, nil, total) // never released
	got, m := renderResidency(notes, total)

	thr := releaseQuietAmp
	loud := lastAbove(ref, thr)
	if loud < 5*sr {
		t.Fatalf("reference tail ends at %d samples: the 1 s delay with feedback 0.7 should ring for seconds", loud)
	}
	for i := 0; i <= loud; i++ {
		if got[i] != ref[i] {
			t.Fatalf("sample %d differs (%g vs %g) before the tail fell below -90 dBFS at %d: the release cut the tail", i, got[i], ref[i], loud)
		}
	}
	if !m.residencyOf(channelForInstrument(channel.Of(id))).released {
		t.Fatal("the unheld channel still runs long after its tail ended")
	}
}

func TestHeldChannelNeverReleases(t *testing.T) {
	id := roster.Snare
	residencyReset(t, id)
	SetInsertEffects(id, longDelay())
	sr := SampleRate()
	_, m := renderResidency([]StageNote{{Inst: id, At: 100, Vol: 1, Dur: 1}}, 40*sr)
	if m.residencyOf(channelForInstrument(channel.Of(id))).released {
		t.Fatal("a held channel was released")
	}
}

func TestRebuiltChannelEqualsFresh(t *testing.T) {
	residencyReset(t)
	id := roster.Snare
	SetInsertEffects(id, []EffectSlot{{Type: EffectDelay, Enabled: true, Params: map[string]float64{"time": 200, "feedback": 0.3, "mix": 0.5}}})
	sr := SampleRate()
	second := 10 * sr
	total := second + 3*sr
	got, _ := renderResidency([]StageNote{{Inst: id, At: 100, Vol: 1, Dur: 1}, {Inst: id, At: second, Vol: 1, Dur: 1}}, total)
	fresh, _ := RenderStagesOffline([]StageNote{{Inst: id, At: second, Vol: 1, Dur: 1}}, nil, total)
	// Released again once its tail drops below the threshold.
	for i := second; i <= lastAbove(fresh, releaseQuietAmp); i++ {
		if got[i] != fresh[i] {
			t.Fatalf("sample %d after re-acquire differs from a fresh channel (%g vs %g)", i, got[i], fresh[i])
		}
	}
}

func TestChainMemoryCoversTheLongestEffect(t *testing.T) {
	if got := ChainMemorySec(longDelay()); got < 1.0 {
		t.Fatalf("a 1 s delay line holds %g s of memory, want ≥ 1", got)
	}
	off := longDelay()
	off[0].Enabled = false
	if got := ChainMemorySec(off); got != 0 {
		t.Fatalf("a disabled effect holds %g s of memory, want 0", got)
	}
}

// An orphaned mixer would keep running every channel from the device callback behind whatever comes next.
func TestResetClosesTheLiveMixer(t *testing.T) {
	Play(roster.Snare)
	old := mix
	if old == nil || old.player == nil {
		t.Skip("no audio device: the live mixer never started")
	}
	Reset()
	if old.player.IsPlaying() {
		t.Fatal("Reset left the previous live mixer's player running")
	}
}
