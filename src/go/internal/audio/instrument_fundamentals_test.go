package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Spelled out, not called, so the "no pitched instrument moved" guarantee is checked against a FROZEN definition.
func legacyPitchOffset(id string) float64 {
	p := MergedInstrumentParamsRO(instrument.ID(id))
	get := func(k string) float64 { return p[k] }
	off := get("osc_octave") * 12
	off += get("osc_detune") / 100.0
	// Through the shared base-frequency list so a param RENAME with the same meaning reads as no change.
	if base, _ := declaredBaseFrequency(p); base > 0 {
		off += 12 * math.Log2(base/220.0)
	}
	return off
}

// The table may only give a note to an instrument that had none, or correct a demonstrably wrong one: letting a
// measurement win would move the melodic roster (violin measures 219.87 Hz against a declared 220).
func TestFundamentalsTableOnlyAddsNoteNames(t *testing.T) {
	const tolSemis = 0.01
	for _, id := range roster.All {
		if !IsMelodicInstrument(id) {
			continue // was unpitched before; gaining a note is the point
		}
		off, pitched := InstrumentPitchInfo(id)
		if !pitched {
			t.Errorf("%s: was pitched before the table, now reports unpitched", id)
			continue
		}
		if want := legacyPitchOffset(string(id)); math.Abs(off-want) > tolSemis {
			t.Errorf("%s: offset moved %+.4f semitones (was %+.4f, now %+.4f); the table must not retune the melodic roster",
				id, off-want, want, off)
		}
	}
}

// A DSP change that strands a family without a note name fails here, not as a blank chip.
func TestFundamentalsTableCoversTheRoster(t *testing.T) {
	var pitched, unpitched []string
	for _, id := range roster.All {
		if _, ok := InstrumentPitchInfo(id); ok {
			pitched = append(pitched, string(id))
		} else {
			unpitched = append(unpitched, string(id))
		}
	}
	// Explicit: adding to it should be a decision, not a side effect.
	wantUnpitched := map[instrument.ID]bool{
		// No fundamental at all: confidence 0.
		roster.Hihat: true, roster.HihatOpenBright: true, roster.HihatLofi: true, roster.HihatPedal: true,
		roster.Clap: true, roster.ClapBright: true, roster.ClapLofi: true, roster.ClapTight: true,
		roster.Shaker: true, roster.VoiceWhisper: true,
		// A dense inharmonic plate is periodic enough for YIN to lock onto a subharmonic at full confidence, with no energy
		// at the frequency it reports; only the spectral-support gate can see that.
		roster.Ride: true, roster.Crash: true,
	}
	for _, id := range unpitched {
		if !wantUnpitched[instrument.ID(id)] {
			t.Errorf("%s: has no note name, but is not one of the known unpitched-noise instruments; "+
				"if that is intended add it to wantUnpitched with a reason", id)
		}
	}
	for id := range wantUnpitched {
		found := false
		for _, u := range unpitched {
			if instrument.ID(u) == id {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected to have no note name (unpitched noise) but it resolved one", id)
		}
	}
	if len(pitched) < 100 {
		t.Errorf("only %d of %d instruments name a note; the point of the table is that this is the common case",
			len(pitched), len(roster.All))
	}
	t.Logf("note names: %d of %d instruments (%d unpitched)", len(pitched), len(roster.All), len(unpitched))
}

// A tuning knob must move the displayed note at once, with no cache to invalidate: why the table stores a BASE and
// the offset is base + delta.
func TestPitchOffsetTracksLiveParamEdits(t *testing.T) {
	// kick-808's base is a MEASUREMENT, not the voice_freq_hz knob, so a naive read would ignore the edit.
	const id = roster.Kick808
	before, pitched := InstrumentPitchInfo(id)
	if !pitched {
		t.Fatalf("%s: expected a note name", id)
	}
	def := libraryFieldValues(id)["voice_freq_hz"]
	if def <= 0 {
		t.Fatalf("%s: expected a positive default voice_freq_hz, got %v", id, def)
	}

	// Tune it up an octave and expect the note to follow by ~12 semitones.
	SetInstrumentParam(id, "voice_freq_hz", def*2)
	t.Cleanup(func() { SetInstrumentParam(id, "voice_freq_hz", def) })

	after, _ := InstrumentPitchInfo(id)
	if got := after - before; math.Abs(got-12) > 0.01 {
		t.Errorf("doubling voice_freq_hz moved the note by %+.4f semitones, want +12.0000 "+
			"(the displayed note is not tracking live synth edits)", got)
	}
}

// Resample (post_pitch) shifts the whole voice, so the note chip follows it.
func TestResampleMovesTheNote(t *testing.T) {
	for _, id := range []instrument.ID{roster.Kick808, roster.FmBell, roster.Violin} {
		before, pitched := InstrumentPitchInfo(id)
		if !pitched {
			t.Fatalf("%s: expected a note name", id)
		}
		SetInstrumentParam(id, "post_pitch", 7)
		after, _ := InstrumentPitchInfo(id)
		ResetInstrumentParams(id)
		if got := after - before; math.Abs(got-7) > 0.01 {
			t.Errorf("%s: Resample +7 st moved the note by %+.4f semitones, want +7", id, got)
		}
	}
}

// fm-bell's row is declared from gen1_fm_base, the knob's current spelling.
func TestFMBaseKnobMovesTheNote(t *testing.T) {
	const id = roster.FmBell
	before, pitched := InstrumentPitchInfo(id)
	if !pitched {
		t.Fatalf("%s: expected a note name", id)
	}
	def := MergedInstrumentParamsRO(id)["gen1_fm_base"]
	if def <= 0 {
		t.Fatalf("%s: expected a positive gen1_fm_base default, got %v", id, def)
	}
	SetInstrumentParam(id, "gen1_fm_base", def*2)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	after, _ := InstrumentPitchInfo(id)
	if got := after - before; math.Abs(got-12) > 0.01 {
		t.Errorf("doubling gen1_fm_base moved the note by %+.4f semitones, want +12", got)
	}
}

// A kick tuned through voice_freq_hz must not display A3 for a voice sounding 44-85 Hz.
func TestModularKicksNoLongerReportA3(t *testing.T) {
	for _, id := range []instrument.ID{roster.Kick808, roster.KickElectro, roster.DnbKick, roster.RawKick, roster.ZgumpKick, roster.KickPunchy} {
		off, pitched := InstrumentPitchInfo(id)
		if !pitched {
			t.Errorf("%s: expected a note name", id)
			continue
		}
		hz := 220 * math.Pow(2, off/12)
		if hz > 110 {
			t.Errorf("%s: resolves to %.1f Hz (%+.2f semitones from A3); a kick belongs below 110 Hz — "+
				"this is the A3 default leaking back in", id, hz, off)
		}
	}
}

// An instance row renders identically to its base, so a different note name would describe the same audio two ways.
func TestInstanceRowsMatchTheirBase(t *testing.T) {
	t.Cleanup(ResetInstruments)
	for _, base := range []instrument.ID{roster.Kick, roster.Snare, roster.Kick808, roster.OrganChurch, roster.Hihat} {
		id := base + "--2"
		EnsureInstanceInstrument(id)

		gotOff, gotPitched := InstrumentPitchInfo(id)
		wantOff, wantPitched := InstrumentPitchInfo(base)
		if gotPitched != wantPitched {
			t.Errorf("%s: pitched=%v but base %s is pitched=%v", id, gotPitched, base, wantPitched)
		}
		if math.Abs(gotOff-wantOff) > 1e-9 {
			t.Errorf("%s: offset %+.4f but base %s is %+.4f — two rows on one sound disagree about its note",
				id, gotOff, base, wantOff)
		}
	}
}
