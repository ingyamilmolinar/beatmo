package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// legacyPitchOffset reproduces the declared-parameter computation
// InstrumentPitchInfo used before the fundamentals table existed. It is kept
// here, spelled out rather than called, so the "no currently-pitched
// instrument moved" guarantee is checked against a FROZEN definition of the
// old behaviour instead of against whatever the current code happens to do.
func legacyPitchOffset(id string) float64 {
	p := MergedInstrumentParamsRO(instrument.ID(id))
	get := func(k string) float64 { return p[k] }
	off := get("osc_octave") * 12
	off += get("osc_detune") / 100.0
	// Resolved through the SHARED base-frequency list rather than three
	// literals. That keeps this a faithful model of the old behaviour across a
	// param RENAME: the config-first migration respelled the FM family's
	// fm_base_freq as gen1_fm_base without changing its meaning or its value,
	// and three frozen literals would have modelled that as "FM lost its
	// declared base" — an artifact of this function, not a real change.
	if base, _ := declaredBaseFrequency(p); base > 0 {
		off += 12 * math.Log2(base/220.0)
	}
	return off
}

// TestFundamentalsTableOnlyAddsNoteNames is the blast-radius guard for making
// note display universal: every instrument that showed a note before must
// still show one, and must still show the SAME one.
//
// The table is allowed to do exactly two things — give a note to an instrument
// that had none, and correct a note that was demonstrably wrong (the modular
// kicks, which reported A3 for a voice sounding 44-85 Hz). It is not allowed to
// silently retune the melodic roster, which is what a measurement-first
// precedence would have done: violin measures 219.87 Hz against a declared
// 220.0, and letting the measurement win would have moved every melodic
// instrument by a hundredth of a semitone for no benefit.
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

// TestFundamentalsTableCoversTheRoster pins what the table actually resolved,
// so a DSP change that quietly strands a family without a note name fails here
// rather than being noticed as a blank chip.
func TestFundamentalsTableCoversTheRoster(t *testing.T) {
	var pitched, unpitched []string
	for _, id := range roster.All {
		if _, ok := InstrumentPitchInfo(id); ok {
			pitched = append(pitched, string(id))
		} else {
			unpitched = append(unpitched, string(id))
		}
	}
	// Every instrument without a note must be genuinely unpitched noise. This
	// list is deliberately explicit: adding to it should be a decision, not a
	// side effect.
	wantUnpitched := map[instrument.ID]bool{
		// Noise with no fundamental at all: nothing to name. The hi-hats, claps,
		// shaker and whisper return confidence 0.000 outright.
		roster.Hihat: true, roster.HihatOpenBright: true, roster.HihatLofi: true, roster.HihatPedal: true,
		roster.Clap: true, roster.ClapBright: true, roster.ClapLofi: true, roster.ClapTight: true,
		roster.Shaker: true, roster.VoiceWhisper: true,
		// The cymbals are the subtle pair, and the reason the spectral-support
		// gate exists. A ride reads 501 Hz at confidence 0.998 and a crash 207 Hz
		// at 1.000 — both fully "reliable" — because a dense inharmonic plate is
		// periodic enough for YIN to lock onto a subharmonic of its mode cluster.
		// Neither has ANY energy at the frequency it reports: the ride's lowest
		// partial sits at 6.0x it, the crash's at 9.7x. Confidence cannot see
		// that; only the spectrum can.
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

// TestPitchOffsetTracksLiveParamEdits is the reason the table stores a BASE and
// the offset is resolved as base+delta rather than being read straight out.
//
// A user turning a tuning knob must move the displayed note immediately, with
// no cache to invalidate — the property InstrumentPitchInfo's contract has
// always promised. Before the table this fell out of reading the params
// directly; now it has to be arranged, so it has to be tested.
func TestPitchOffsetTracksLiveParamEdits(t *testing.T) {
	// kick-808 is the sharpest case: its base comes from a MEASUREMENT (50 Hz),
	// not from the voice_freq_hz knob, so a naive implementation would read the
	// measured value and ignore the edit entirely.
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

// TestResampleMovesTheNote: the FINISH stage's Resample (post_pitch) shifts the
// whole rendered voice by its semitones, so the note chip has to follow it —
// the preview and the focus graph already treat it as pitch.
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

// TestFMBaseKnobMovesTheNote is the FM family's half of the live-edit contract.
// fm-bell's table row is DECLARED from gen1_fm_base (the config-first spelling
// of the FM "Base Pitch" knob), but the pitch scanner only read the retired
// fm_base_freq spelling — so turning Base Pitch changed the sound and left the
// note chip where it was.
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

// TestModularKicksNoLongerReportA3 pins the specific bug this change exists to
// fix: every modular kick is tuned through voice_freq_hz, which the pitch
// scanner did not read, so a voice sounding 44-85 Hz displayed as A3 (220 Hz) —
// a two-octave error on the instruments a drum machine is most about.
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

// TestInstanceRowsMatchTheirBase pins that a duplicated row names the same note
// as the row it was duplicated from.
//
// gen-showcase mints "<id>--N" for the Nth copy of an instrument in a template
// (a keyboard part split into chord lanes routinely produces three), and
// EnsureInstanceInstrument promotes those to render IDENTICALLY to the base. A
// note name that disagreed would therefore be describing the same audio two
// ways: before the instance resolution in InstrumentPitchInfo, a duplicated
// kick row showed NO note beside an original showing A1, and organ-church--2
// reported +0 semitones against its base's -12 — a full octave.
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
