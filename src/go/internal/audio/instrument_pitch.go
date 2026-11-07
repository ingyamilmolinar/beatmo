package audio

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The values the fundamentals table was measured at.
type pitchDefaults struct {
	oscOctave   float64
	oscDetune   float64
	fmBase      float64
	voiceFreqHz float64
	postPitch   float64 // FINISH Resample, semitones
}

// Keyed by the library revision.
type libraryFieldsEntry struct {
	libRev uint64
	vals   map[string]float64
}

var (
	libraryFieldsMu    sync.RWMutex
	libraryFieldsCache = map[instrument.ID]libraryFieldsEntry{}
)

// nil when id is not in the library.
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

// Semitones from A3 (node pitch 0) that the instrument's live document and Sampler edit add, and whether it names a
// note. Each pitch knob is resolved as the render resolves it, shipped value plus edit, since a shipped octave moves
// the sound as a user knob would. Node pitch is not folded in. Allocation-free on the cached path: called every frame.
func InstrumentPitchInfo(instrumentID instrument.ID) (offsetSemis float64, pitched bool) {
	// An instance row renders IDENTICALLY to its base, so it must name the same note; the instance id still drives the
	// params lookup, so a per-instance edit keeps applying.
	baseID := instrumentID.Base()

	// The "defaults" are the library document the table was measured at, so the delta is exactly what edits added.
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

	// The base comes from the generated table: rendering and measuring is the only answer for families whose pitch lives
	// in a pitch envelope rather than a knob.
	declaresFundamental := false
	base, known := instrumentFundamentals[instrumentID]
	if !known && baseID != instrumentID {
		base, known = instrumentFundamentals[baseID]
	}

	if known && base.baseHz > 0 {
		// A delta over the measured defaults, not an absolute read, so the note follows a tuning knob with nothing cached.
		offsetSemis += 12 * math.Log2(base.baseHz/220.0)
		offsetSemis += (pick(oOct, d.oscOctave) - d.oscOctave) * 12
		offsetSemis += (pick(oDet, d.oscDetune) - d.oscDetune) / 100.0
		offsetSemis += pick(oPost, d.postPitch) - d.postPitch
		// FM declares gen1_fm_base, the other families voice_freq_hz; the order matches baseFrequencyParamNames.
		offsetSemis += baseFreqDeltaSemis(
			firstPositive(pick(oFM, d.fmBase), pick(oVoice, d.voiceFreqHz)),
			firstPositive(d.fmBase, d.voiceFreqHz),
		)
	} else {
		// Not in the table (a user instrument, or one resolved as unpitched): the declared-parameter computation, so such an
		// instrument keeps its answer.
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

	hasSampleEdit := false
	if e, ok := SampleEditFor(instrumentID); ok {
		offsetSemis += e.TransposeSemis + e.DetuneCents/100.0
		hasSampleEdit = true
	}

	// The melodic flag keeps the table from REMOVING a note name (an unreliable measurement on a melodic voice); a declared
	// fundamental covers ids the table has no row for, or a user's copy of a drum would lose a note its original names.
	pitched = (known && base.baseHz > 0) || declaresFundamental || instrumentDocMelodic(instrumentID) || hasSampleEdit
	return offsetSemis, pitched
}

// Zero when either value is absent.
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
