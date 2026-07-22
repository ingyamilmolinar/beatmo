package ui

import (
	"errors"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// errInstrumentRow is returned when a bundle build/apply targets a row index
// outside the current row set.
var errInstrumentRow = errors.New("instrument bundle: row index out of range")

// wavRegistry lazily returns the DrumView's WAV version registry.
func (dv *DrumView) wavRegistry() *audio.WavRegistry {
	if dv.wavReg == nil {
		dv.wavReg = audio.NewWavRegistry()
	}
	return dv.wavReg
}

// BuildInstrumentBundle assembles a portable, config-only bundle for ONE
// instrument (the row at rowIdx). It reconstructs a shareable recipe (base +
// seed), normalizes the row's insert-effects into the bundle's effect library,
// carries the optional per-instrument EQ inline, and — for a user WAV — records
// a reference (name + content hash + source locations), never the PCM.
//
// The result is a 1-instrument bundle; the same builder over a set produces a
// multi-instrument "pack" (identical schema, shared effects library).
func (dv *DrumView) BuildInstrumentBundle(rowIdx int) (audio.InstrumentBundle, error) {
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		return audio.InstrumentBundle{}, errInstrumentRow
	}
	row := dv.Rows[rowIdx]
	instID := row.Instrument

	recipeDoc, err := audio.BuildInstrumentRecipeDoc(instID)
	if err != nil {
		return audio.InstrumentBundle{}, err
	}

	doc := audio.InstrumentDoc{
		Name:   row.Name,
		Recipe: recipeDoc,
		Kind:   "synth",
	}
	if c := hexColor(row.Color); c != "" {
		doc.Color = c
	}

	// Sample-edit (non-destructive) → kind "sample".
	if e, ok := audio.SampleEditFor(instID); ok {
		doc.SampleEdit = e.Fields()
		doc.Kind = "sample"
	}

	// User WAV → kind "wav" + a hash-pinned reference (NO PCM). The pin is the
	// content hash of the CANONICAL WAV bytes (the stored PCM re-encoded through
	// the recording encoder) — a deterministic, decoder-independent "file
	// version". Sources point at where those bytes can be re-fetched.
	if audio.IsUserSample(instID) {
		doc.Kind = "wav"
		if rec, ok := audio.UserSamplePCM(instID); ok {
			if canonical, err := wavgen.EncodeWAV(rec.PCM, rec.SampleRate); err == nil {
				sha := audio.Sha256Hex(canonical)
				v := dv.wavRegistry().Register(instID, sha)
				ref := &audio.WavRef{Name: instID + ".wav", Sha256: sha, Version: v.Version}
				// The sample store serves this WAV keyed by instrument id on both
				// platforms (its PCM re-encodes to the same canonical bytes).
				ref.Sources = append(ref.Sources, audio.WavSource{Kind: audio.WavSourceLocalStorage, Ref: instID})
				doc.WavRef = ref
			}
		}
	}

	// Effects → normalized library + ordered id chain.
	defs, chain := audio.NormalizeEffects(audio.GetInsertEffects(instID))
	doc.EffectChain = chain

	// Optional inline EQ (only when it actually shapes the sound).
	if eq := buildInstrumentEQ(row); eq != nil {
		doc.EQ = eq
	}

	return audio.InstrumentBundle{
		Format:      audio.InstrumentBundleFormat,
		Version:     audio.InstrumentBundleVersion,
		Effects:     defs,
		Instruments: []audio.InstrumentDoc{doc},
	}, nil
}

// buildInstrumentEQ snapshots a row's EQ into a portable struct, or nil when the
// EQ is flat and all filters are off (nothing worth carrying).
func buildInstrumentEQ(row *DrumRow) *audio.InstrumentEQ {
	active := row.HPFEnabled || row.LPFEnabled
	for _, g := range row.EQGainsDB {
		if math.Abs(g) > 1e-9 {
			active = true
			break
		}
	}
	for _, m := range row.EQBandMuted {
		if m {
			active = true
			break
		}
	}
	if !active {
		return nil
	}
	eq := &audio.InstrumentEQ{
		HPFEnabled:  row.HPFEnabled,
		HPFCutoffHz: row.HPFCutoffHz,
		LPFEnabled:  row.LPFEnabled,
		LPFCutoffHz: row.LPFCutoffHz,
	}
	if len(row.EQGainsDB) > 0 {
		eq.GainsDB = append([]float64(nil), row.EQGainsDB...)
	}
	if len(row.EQBandMuted) > 0 {
		eq.BandMuted = append([]bool(nil), row.EQBandMuted...)
	}
	return eq
}
