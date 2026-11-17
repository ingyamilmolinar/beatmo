package audio

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// pitchDefaults holds the pitch-bearing subset of an instrument's LIBRARY
// document — the values the fundamentals table was measured at.
type pitchDefaults struct {
	oscOctave   float64
	oscDetune   float64
	fmBase      float64
	voiceFreqHz float64
	postPitch   float64 // FINISH Resample, semitones
}

// libraryFieldsEntry caches the flat view of a library document (the values
// the fundamentals table was measured at), keyed by the library revision.
type libraryFieldsEntry struct {
	libRev uint64
	vals   map[string]float64
}

var (
	libraryFieldsMu    sync.RWMutex
	libraryFieldsCache = map[instrument.ID]libraryFieldsEntry{}
)

// libraryFieldValues is the flat view (FieldValues) of library document id,
// recomputed only when the library changes. nil when id is not in the library.
func libraryFieldValues(id instrument.ID) map[string]float64 {
	rev := libraryRev.Load()
	libraryFieldsMu.RLock()
	e, ok := libraryFieldsCache[id]
	libraryFieldsMu.RUnlock()
	if ok && e.libRev == rev {
		return e.vals
	}
	var vals map[string]float64
	if d, ok := LibraryDoc(id); ok {
		vals = instrument.FieldValues(d, BuiltinModuleRegistry())
	}
	libraryFieldsMu.Lock()
	libraryFieldsCache[id] = libraryFieldsEntry{libRev: rev, vals: vals}
	libraryFieldsMu.Unlock()
	return vals
}

// InstrumentPitchInfo reports the effective pitch offset (in semitones from
// A3 = 220 Hz, the node-pitch-0 convention) contributed by an instrument's
// current synth + sampler parameters, and whether the instrument is pitched
// (musically noteworthy) rather than unpitched percussion.
//
// It is the note-display companion to the render path: a node's absolute note
// is noteName(A3 + nodePitch + offsetSemis). Because it reads the live param +
// sampler-edit state, the note derived from it tracks synth/sampler edits
// automatically — no cached value to keep in sync.
//
// It resolves each pitch knob the SAME WAY the render path does — from the
// instrument's document, whatever it ships with plus any edit — because a
// pitch-shifting value the instrument ships with
// moves the sounding frequency exactly as a user knob would. Reading only the
// override map (as this did before 2026-09-12) reported +0 semitones for the
// thirteen instruments whose seeds carry an octave or a detune, so the
// displayed note disagreed with the sounding one by up to a full octave, and
// the score importer — which has no other programmatic answer to "does this
// instrument shift pitch?" — hand-maintained the same knowledge in a comment
// that drifted (double-bass was compensated for an octave offset it does not
// have; voice-soprano's real +1 octave was compensated nowhere).
//
// Node pitch itself is NOT folded in here: at render time the node's semitone
// pitch overrides the recipe's generic "pitch" param (synth_recipe_dispatch.go
// merged["pitch"] = pitch), so the caller adds node pitch exactly once on top
// of this offset. The knobs folded here are the ones that STACK on top of the
// A3 base: octave, fine-detune (cents), and a base-frequency override.
//
// Performance: allocation-free on the cached path and holds each lock only
// briefly, so it is safe to call every frame from the UI draw loop. It reads
// the specific pitch keys directly without cloning the params map (unlike
// GetInstrumentParams, which returns a defensive copy) and reads the recipe
// defaults through a revision-gated per-recipe cache.
func InstrumentPitchInfo(instrumentID instrument.ID) (offsetSemis float64, pitched bool) {
	// An instance row ("kick--2", the suffix gen-showcase mints for the Nth
	// copy of an instrument in a template) must answer exactly as its base:
	// EnsureInstanceInstrument promotes it to render IDENTICALLY, so anything
	// else here means two rows playing one sound disagree about what note it
	// is. Before this resolution they did — a duplicated kick row showed no
	// note beside an original showing A1, and organ-church--2 reported +0
	// against its base's -12, a full octave out.
	//
	// The INSTANCE id still drives the params lookup, so a per-instance
	// override keeps overriding; only the recipe and the fundamentals entry
	// fall back to the base.
	baseID := instrumentID.Base()

	// The live values are the instrument's project document; the "defaults"
	// are the library document it came from — the values the fundamentals table
	// was measured at — so the delta is exactly what the user's edits added.
	cur := MergedInstrumentParamsRO(instrumentID)
	origin := baseID
	if o := instrumentDocOrigin(instrumentID); o != "" {
		origin = o
	}
	lib := libraryFieldValues(origin)
	if lib == nil {
		lib = cur
	}
	type ov struct {
		val float64
		ok  bool
	}
	get := func(k string) ov {
		v, ok := cur[k]
		return ov{v, ok}
	}
	oOct, oDet := get("osc_octave"), get("osc_detune")
	oFM, oVoice := get("gen1_fm_base"), get("voice_freq_hz")
	oPost := get("post_pitch")
	d := pitchDefaults{oscOctave: lib["osc_octave"], oscDetune: lib["osc_detune"],
		fmBase: lib["gen1_fm_base"], postPitch: lib["post_pitch"], voiceFreqHz: lib["voice_freq_hz"]}
	pick := func(o ov, def float64) float64 {
		if o.ok {
			return o.val
		}
		return def
	}

	// The instrument's BASE fundamental — what it sounds at its shipped
	// parameters — comes from the generated table (cmd/gen-fundamentals), which
	// resolves it by rendering the voice and measuring it, falling back to a
	// declared base-frequency param. That is the only way to answer the
	// question for the families whose pitch lives in a pitch envelope rather
	// than in any single declared knob.
	declaresFundamental := false
	base, known := instrumentFundamentals[instrumentID]
	if !known && baseID != instrumentID {
		base, known = instrumentFundamentals[baseID]
	}

	if known && base.baseHz > 0 {
		// Absolute base from the table, then the DELTA the live parameters add
		// on top of the defaults it was measured at. Reading the delta rather
		// than the absolute value is what keeps the note tracking synth edits:
		// turn the tuning knob and the note moves, with nothing cached here to
		// invalidate.
		offsetSemis += 12 * math.Log2(base.baseHz/220.0)
		offsetSemis += (pick(oOct, d.oscOctave) - d.oscOctave) * 12
		offsetSemis += (pick(oDet, d.oscDetune) - d.oscDetune) / 100.0
		offsetSemis += pick(oPost, d.postPitch) - d.postPitch
		// A base-frequency knob moved away from the default the table was
		// measured at shifts the pitch by their ratio. The FM family declares
		// gen1_fm_base, the other families voice_freq_hz; the order matches
		// baseFrequencyParamNames.
		offsetSemis += baseFreqDeltaSemis(
			firstPositive(pick(oFM, d.fmBase), pick(oVoice, d.voiceFreqHz)),
			firstPositive(d.fmBase, d.voiceFreqHz),
		)
	} else {
		// Not in the table: a user instrument, or one the table resolved as
		// unpitched. Fall back to the declared-parameter computation, which is
		// exactly what this function did before the table existed — so an
		// instrument the table has nothing to say about keeps its old answer.
		offsetSemis += pick(oOct, d.oscOctave) * 12
		offsetSemis += pick(oDet, d.oscDetune) / 100.0
		offsetSemis += pick(oPost, d.postPitch)
		if baseFreq := firstPositive(
			pick(oFM, d.fmBase),
			pick(oVoice, d.voiceFreqHz),
		); baseFreq > 0 {
			offsetSemis += 12 * math.Log2(baseFreq/220.0)
			declaresFundamental = true
		}
	}

	// Sampler transpose/detune, if the instrument carries a sample-edit
	// descriptor (a non-destructive resample applied at trigger time).
	hasSampleEdit := false
	if e, ok := SampleEditFor(instrumentID); ok {
		offsetSemis += e.TransposeSemis + e.DetuneCents/100.0
		hasSampleEdit = true
	}

	// An instrument is "pitched" (worth a musical note name) when we can say
	// what note it sounds. Since 2026-09-13 that is the common case rather than
	// the curated melodic one: the table resolves a fundamental for 119 of the
	// 133 built-ins, so kicks, snares, toms, congas, cowbell and the FM family
	// all name a note, and only genuinely unpitched noise (hi-hats, claps,
	// shaker, whisper) declines to.
	//
	// pitchAwareRecipe stays in the disjunction so the table can only ADD note
	// names, never remove one: a melodic instrument whose measurement came back
	// unreliable (tubular-bells, voice-ahh, voice-opera) keeps the note it has
	// always shown via the declared-parameter fallback above.
	// declaresFundamental covers the ids the table has no row for — a user
	// instrument, or one built on a drum recipe — whose recipe nonetheless
	// declares a base frequency. Without it the rule would be "shipped drums
	// name a note, a user's copy of one does not", which is a distinction the
	// user has no way to see the logic of.
	pitched = (known && base.baseHz > 0) || declaresFundamental || instrumentDocMelodic(instrumentID) || hasSampleEdit
	return offsetSemis, pitched
}

// baseFreqDeltaSemis is the semitone shift from moving a base-frequency knob
// off the default the fundamentals table was measured at. Zero when either
// value is absent, so an instrument with no such knob contributes nothing.
func baseFreqDeltaSemis(now, def float64) float64 {
	if now <= 0 || def <= 0 || now == def {
		return 0
	}
	return 12 * math.Log2(now/def)
}

// firstPositive returns the first strictly-positive argument, or 0 if none is.
func firstPositive(vals ...float64) float64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}
