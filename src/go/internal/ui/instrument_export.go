package ui

import (
	"errors"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

var errInstrumentRow = errors.New("instrument bundle: row index out of range")

// wavRegistry lazily returns the DrumView's WAV version registry.
func (dv *DrumView) wavRegistry() *audio.WavRegistry {
	if dv.wavReg == nil {
		dv.wavReg = audio.NewWavRegistry()
	}
	return dv.wavReg
}

// Config only: a user WAV becomes a reference (name, content hash, sources), never PCM. The same builder
// over a set makes a multi-instrument pack.
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

	// The pin is the hash of the CANONICAL WAV bytes (stored PCM re-encoded), a decoder-independent version.
	if audio.IsUserSample(instID) {
		if rec, ok := audio.UserSamplePCM(instID); ok {
			if canonical, err := wavgen.EncodeWAV(rec.PCM, rec.SampleRate); err == nil {
				sha := audio.Sha256Hex(canonical)
				v := dv.wavRegistry().Register(string(instID), sha)
				ref := &audio.WavRef{Name: string(instID) + ".wav", Sha256: sha, Version: v.Version}
				// Its PCM re-encodes to the same canonical bytes on both platforms.
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

// nil when flat with filters off.
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
