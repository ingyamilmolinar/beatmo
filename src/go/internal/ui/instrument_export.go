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

	doc := audio.BundleInstrument{Name: row.Name}
	if synth, ok := audio.InstrumentDoc(instID); ok {
		synth.Origin = ""
		doc.Synth = &synth
	}
	if c := hexColor(row.Color); c != "" {
		doc.Color = c
	}

	// Sample-edit (non-destructive).
	if e, ok := audio.SampleEditFor(instID); ok {
		doc.SampleEdit = e.Fields()
	}

	// User WAV → a hash-pinned reference (NO PCM). The pin is the
	// content hash of the CANONICAL WAV bytes (the stored PCM re-encoded through
	// the recording encoder) — a deterministic, decoder-independent "file
	// version". Sources point at where those bytes can be re-fetched.
	if audio.IsUserSample(instID) {
		if rec, ok := audio.UserSamplePCM(instID); ok {
			if canonical, err := wavgen.EncodeWAV(rec.PCM, rec.SampleRate); err == nil {
				sha := audio.Sha256Hex(canonical)
				v := dv.wavRegistry().Register(string(instID), sha)
				ref := &audio.WavRef{Name: string(instID) + ".wav", Sha256: sha, Version: v.Version}
				// The sample store serves this WAV keyed by instrument id on both
				// platforms (its PCM re-encodes to the same canonical bytes).
				ref.Sources = append(ref.Sources, audio.WavSource{Kind: audio.WavSourceLocalStorage, Ref: string(instID)})
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
	doc.Kind = doc.KindOf()

	bundle := audio.NewInstrumentBundle()
	bundle.Effects = defs
	bundle.Instruments = []audio.BundleInstrument{doc}
	return bundle, nil
}

// buildInstrumentEQ snapshots a row's EQ into a portable struct, or nil when the
// EQ is flat and all filters are off (nothing worth carrying).
func buildInstrumentEQ(row *DrumRow) *audio.InstrumentEQ {
	src := rowEQ(row)
	active := src.HPF || src.LPF
	for _, g := range src.GainsDB {
		if math.Abs(g) > 1e-9 {
			active = true
			break
		}
	}
	for _, m := range src.BandMuted {
		if m {
			active = true
			break
		}
	}
	if !active {
		return nil
	}
	return &audio.InstrumentEQ{
		HPFEnabled:  src.HPF,
		HPFCutoffHz: src.HPFHz,
		LPFEnabled:  src.LPF,
		LPFCutoffHz: src.LPFHz,
		GainsDB:     append([]float64(nil), src.GainsDB...),
		BandMuted:   append([]bool(nil), src.BandMuted...),
	}
}
