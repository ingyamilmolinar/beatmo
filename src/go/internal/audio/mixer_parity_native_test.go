//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The desktop mixer's half of the cross-platform contract
// (xplat_audio_stages.browser.test.js holds the browser to the same numbers):
// a note is its played voice × volume × headroom with a linear fade-in, from
// its own sample; a lane stop fades linearly from its own sample; a channel's
// effects ring on without voices; a recording at another rate is converted.

func stageReset(t *testing.T) {
	t.Helper()
	ClearAllInsertEffectsAndNotify()
	for _, id := range roster.All {
		ClearChannelProcessors(channel.Of(id))
	}
	ClearChannelProcessors(channel.Master)
	prev := MainVolume()
	SetMainVolume(1)
	t.Cleanup(func() { SetMainVolume(prev); ClearAllInsertEffectsAndNotify() })
}

// TestEngineRunsAt48k: desktop runs at the browser's rate.
func TestEngineRunsAt48k(t *testing.T) {
	if SampleRate() != EngineSampleRate || EngineSampleRate != 48000 {
		t.Fatalf("engine at %d Hz, EngineSampleRate %d: both platforms run at 48 kHz", SampleRate(), EngineSampleRate)
	}
}

// TestMixerVoiceIsThePlayedVoice: from a start sample inside a 64-frame
// block, the mixer's per-instrument signal is exactly the played voice
// (RenderAsPlayed) × volume, faded in linearly over FadeInMs, × headroom —
// for a pitched, shortened drum and a gated, fractional-pitch melodic note.
func TestMixerVoiceIsThePlayedVoice(t *testing.T) {
	stageReset(t)
	sr := SampleRate()
	fadeIn := sr * FadeInMs / 1000
	for _, tc := range []struct {
		id              instrument.ID
		pitch, dur, vol float64
	}{
		{roster.Snare, 3.3, 0.75, 0.37},
		{roster.Banjo, -7.3, 0.5, 0.025},
	} {
		const at = 1000 + 37
		played, _ := RenderAsPlayed(tc.id, tc.pitch, tc.dur, 3)
		total := at + len(played) + 64
		_, taps := RenderStagesOffline([]StageNote{{Inst: tc.id, At: at, Pitch: tc.pitch, Vol: tc.vol, Dur: tc.dur}}, nil, total)
		got := taps[tc.id].Voices
		for i := range got {
			want := 0.0
			if k := i - at; k >= 0 && k < len(played) {
				want = played[k] * tc.vol
				if k < fadeIn {
					want *= float64(k) / float64(fadeIn)
				}
				want *= VoiceHeadroom
			}
			if got[i] != want {
				t.Fatalf("%s: sample %d = %g, the played voice says %g (a note starts on its own sample and is never cut by a block)", tc.id, i, got[i], want)
			}
		}
	}
}

// TestLaneStopLandsOnItsSample: a mute node's stop fades a lane voice from
// the stop's own sample, linearly to zero over StopFadeMs (it used to wait for
// the block containing it and fade quadratically).
func TestLaneStopLandsOnItsSample(t *testing.T) {
	stageReset(t)
	sr := SampleRate()
	id := roster.OrganChurch
	const at, stop = 517, 517 + 9000 + 21
	n := []StageNote{{Inst: id, At: at, Vol: 0.8, Dur: 1, Lane: true}}
	total := stop + sr/10
	_, free := RenderStagesOffline(n, nil, total)
	_, stopped := RenderStagesOffline(n, []StageStop{{Inst: id, At: stop}}, total)
	a, b := free[id].Voices, stopped[id].Voices
	fade := sr * StopFadeMs / 1000
	for i := 0; i < total; i++ {
		want := a[i]
		if i >= stop {
			want = 0
			if k := i - stop; k < fade {
				want = a[i] * (1 - float64(k)/float64(fade))
			}
		}
		if math.Abs(b[i]-want) > 1e-12 {
			t.Fatalf("sample %d (stop at %d) = %g, want %g", i, stop, b[i], want)
		}
	}
}

// TestEffectTailsRingWithoutVoices: a delay's echoes keep sounding after the
// note that fed them has ended — a channel with an effect runs every block,
// voice or not, as the browser's worklet does.
func TestEffectTailsRingWithoutVoices(t *testing.T) {
	stageReset(t)
	sr := SampleRate()
	id := roster.Snare
	SetInsertEffects(id, []EffectSlot{{Type: EffectDelay, Enabled: true, Params: map[string]float64{"time": 300, "feedback": 0.6, "mix": 0.5}}})
	played, _ := RenderAsPlayed(id, 0, 1, 3)
	end := 100 + len(played)
	total := end + 2*sr
	_, taps := RenderStagesOffline([]StageNote{{Inst: id, At: 100, Vol: 1, Dur: 1}}, nil, total)
	var tail float64
	for _, x := range taps[id].Inserts[end+sr/10:] {
		tail = math.Max(tail, math.Abs(x))
	}
	if tail < 1e-3 {
		t.Fatalf("after the voice ended the delay went silent (peak %g): its echoes stopped with the voice", tail)
	}
}

// TestForeignRateSampleIsResampled: PCM recorded at 44.1 or 96 kHz plays at
// its own pitch and length (it used to be registered as-is — 48 kHz PCM
// played 1.47 semitones low — or replaced by silence).
func TestForeignRateSampleIsResampled(t *testing.T) {
	const f = 440.0
	for _, from := range []int{44100, 96000, 22050} {
		pcm := make([]float32, from/2) // 0.5 s
		for i := range pcm {
			pcm[i] = float32(0.5 * math.Sin(2*math.Pi*f*float64(i)/float64(from)))
		}
		id := instrument.ID("xplat-rate-probe")
		RegisterSamplePCM(id, pcm, from)
		v, _ := RenderAsPlayed(id, 0, 1, 1)
		UnregisterSamplePCM(id)
		sr := SampleRate()
		if want := int(math.Round(float64(len(pcm)) * float64(sr) / float64(from))); math.Abs(float64(len(v)-want)) > 1 {
			t.Fatalf("%d Hz: %d frames at %d Hz, want %d (0.5 s)", from, len(v), sr, want)
		}
		crossings := 0
		for i := 1; i < len(v); i++ {
			if (v[i-1] < 0) != (v[i] < 0) {
				crossings++
			}
		}
		got := float64(crossings) / 2 / (float64(len(v)) / float64(sr))
		if cents := 1200 * math.Log2(got/f); math.Abs(cents) > 5 {
			t.Fatalf("%d Hz: the sine plays at %.1f Hz (%.1f cents off)", from, got, cents)
		}
	}
}
