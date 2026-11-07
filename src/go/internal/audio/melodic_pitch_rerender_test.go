//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Drums are pitched by resampling one cached buffer: two kick voices at +12 are byte-identical and the cache key
// carries no pitch.
func TestMelodicPitchRerender_DrumsByteIdentical(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Kick) })
	ResetInstrumentParams(roster.Kick)

	// The kick is not melodic.
	if IsMelodicInstrument(roster.Kick) {
		t.Fatal("IsMelodicInstrument(kick) = true; want false for a drum")
	}

	// The edit forces the recipe path so the cache is hit.
	SetInstrumentParam(roster.Kick, "amp_decay", 0.4) // force recipe path so we hit cache
	t.Cleanup(func() { ResetInstrumentParams(roster.Kick) })

	v1 := newRecipeAwareVoicePitched(roster.Kick, 44100, 12.0)
	v2 := newRecipeAwareVoicePitched(roster.Kick, 44100, 12.0)
	if v1 == nil || v2 == nil {
		t.Fatal("newRecipeAwareVoicePitched returned nil for kick")
	}
	buf1 := drainVoiceToBuffer(v1, 44100)
	buf2 := drainVoiceToBuffer(v2, 44100)
	if len(buf1) == 0 || len(buf2) == 0 {
		t.Fatalf("empty buffers: |buf1|=%d |buf2|=%d", len(buf1), len(buf2))
	}
	if len(buf1) != len(buf2) {
		t.Fatalf("buffer lengths differ: %d vs %d", len(buf1), len(buf2))
	}
	for i := range buf1 {
		if buf1[i] != buf2[i] {
			t.Errorf("drum kick byte-identity failed at sample %d: %v vs %v ("+
				"kick must be unaffected by per-pitch re-render feature)", i, buf1[i], buf2[i])
			break
		}
	}

	if IsMelodicInstrument(roster.Kick) {
		t.Error("IsMelodicInstrument(kick) must be false to guarantee byte-identical drum cache keys")
	}
}

// A violin rendered at +12 must DIFFER from its pitch-0 render resampled ×2: re-rendering keeps the formant where
// resampling would slide it.
func TestMelodicPitchRerender_ViolinReRenderDiffersFromResample(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Violin) })

	const sr = 44100

	// 1. Render violin at pitch=0.
	v0, ok0 := tryRecipeVoicePitched(roster.Violin, sr, 0)
	if !ok0 || v0 == nil {
		t.Fatal("tryRecipeVoicePitched(violin) returned nil/false")
	}
	buf0 := drainVoiceToBuffer(v0, sr)
	if len(buf0) == 0 {
		t.Fatal("empty violin pitch=0 buffer")
	}

	// 2. Render violin at pitch=+12 (octave up, re-rendered in C).
	v12, ok12 := tryRecipeVoicePitched(roster.Violin, sr, 12)
	if !ok12 || v12 == nil {
		t.Fatal("tryRecipeVoicePitched(violin) returned nil/false")
	}
	buf12 := drainVoiceToBuffer(v12, sr)
	if len(buf12) == 0 {
		t.Fatal("empty violin pitch=+12 buffer")
	}

	resampleBuf := resampleBuffer(buf0, 2.0)

	// Identical would mean resampling under the hood.
	n := len(buf12)
	if len(resampleBuf) < n {
		n = len(resampleBuf)
	}
	var l1dist float64
	for i := 0; i < n; i++ {
		l1dist += math.Abs(float64(buf12[i]) - float64(resampleBuf[i]))
	}
	if l1dist == 0 {
		t.Error("violin at-pitch render is identical to resample — " +
			"pitch re-render has no effect (formant not fixed)")
	}
	t.Logf("violin pitch=+12 re-render vs resample L1 dist = %.4f (over %d samples); "+
		"non-zero proves formant is preserved by re-rendering, not resampled", l1dist, n)
}

func TestPitchAwareRecipe_MelodicTrue(t *testing.T) {
	melodic := []instrument.ID{
		roster.Violin,
		roster.ViolinEnsemble,
		roster.Cello,
		roster.CelloWarm,
		roster.GuitarNylon,
		roster.GuitarNylonBright,
		roster.GuitarSteel,
		roster.GuitarSteelWarm,
		roster.GuitarElectric,
		roster.GuitarElectricNeck,
		roster.PianoGrand,
		roster.PianoFelt,
		roster.Flute,
		roster.FluteBreathy,
		roster.Oboe,
		roster.OboeFull,
		roster.Trumpet,
		roster.TrumpetMellow,
		roster.FrenchHorn,
		roster.FrenchHornLoud,
	}
	for _, id := range melodic {
		if !IsMelodicInstrument(id) {
			t.Errorf("IsMelodicInstrument(%q) = false; want true", id)
		}
	}
}

func TestPitchAwareRecipe_NonMelodicFalse(t *testing.T) {
	nonMelodic := []instrument.ID{
		roster.Kick, roster.Snare, roster.Hihat, roster.Clap, roster.Tom,
		roster.Cowbell, roster.SubBass,
		roster.FmBass, roster.FmBell, roster.FmLead, roster.FmEpiano, roster.FmPluck,
		roster.Modular,
		roster.ModularPad,
	}
	for _, id := range nonMelodic {
		if IsMelodicInstrument(id) {
			t.Errorf("IsMelodicInstrument(%q) = true; want false", id)
		}
	}
}

// Different pitches must be distinct cache entries.
func TestMelodicPitchRerender_CacheKeyIncludesPitch(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Violin) })

	const sr = 44100
	vc := newTestCache()

	// Manufacture two keys at different pitches.
	k0 := voiceCacheKey{instrumentID: roster.Violin, sampleRate: sr, paramsHash: 0, pitch: 0}
	k12 := voiceCacheKey{instrumentID: roster.Violin, sampleRate: sr, paramsHash: 0, pitch: 12}

	vc.Put(k0, []float32{1.0})
	vc.Put(k12, []float32{2.0})

	got0, ok0 := vc.Get(k0)
	got12, ok12 := vc.Get(k12)
	if !ok0 || !ok12 {
		t.Fatalf("Get returned false: ok0=%v ok12=%v", ok0, ok12)
	}
	if len(got0) == 0 || len(got12) == 0 {
		t.Fatal("empty cache buffers")
	}
	if got0[0] == got12[0] {
		t.Error("pitch=0 and pitch=12 returned same cache entry — pitch is not in the cache key")
	}
}

// Linear interpolation, like resampleVoice.
func resampleBuffer(src []float32, rate float64) []float32 {
	if len(src) == 0 || rate <= 0 {
		return nil
	}
	// Estimate output length: at rate=2 the output is ~half as long.
	outLen := int(float64(len(src)) / rate)
	if outLen < 1 {
		outLen = 1
	}
	out := make([]float32, 0, outLen)
	pos := 0.0
	for {
		i0 := int(pos)
		if i0 >= len(src)-1 {
			break
		}
		frac := pos - float64(i0)
		s := float32(float64(src[i0])*(1-frac) + float64(src[i0+1])*frac)
		out = append(out, s)
		pos += rate
	}
	return out
}
