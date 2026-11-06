package ui

import (
	"math"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Spectrum findings — the interpretation layer behind the Spectrum tab's
// headline sentence.
//
// DESIGN RULE, and the reason this file is small: a finding may only state
// something CHECKABLE from the data. It never expresses taste. There is no
// target curve and no "your mix should look like X", because a drum machine has
// no correct spectrum — a bass-only circuit is a legitimate sound, not a fault.
// Every kind below is a fact you could verify by hand from the same numbers:
//
//	clipping   — the row's own clip counter fired
//	collision  — two audible rows sound their fundamental in one band
//	pile-up    — a band is far louder than ITS OWN NEIGHBOURS (self-relative)
//	dead range — an end of the spectrum holds essentially nothing
//
// Deliberately NOT here: anything needing a per-row FFT. Only the master and
// the single selected channel carry spectra (analyzer.ChannelMetrics), so
// per-row spectra would mean an FFT per row per analyzer tick — the exact cost
// the Spectrum tab was just relieved of. Attribution instead comes from three
// things that already exist and are cheap: the master FFT (where the energy
// is), per-row RMS/clip counters (who is audible now), and each instrument's
// sounding fundamental (which band a row occupies).
type findingKind int

const (
	findingClipping findingKind = iota
	findingCollision
	findingPileUp
	findingDeadRange
)

func (k findingKind) String() string {
	switch k {
	case findingClipping:
		return "clipping"
	case findingCollision:
		return "collision"
	case findingPileUp:
		return "pile-up"
	case findingDeadRange:
		return "dead-range"
	}
	return "?"
}

// finding is one checkable observation about the current sound.
type finding struct {
	Kind findingKind
	// Band is the ISO band index the finding points at, or -1 when the finding
	// is not band-scoped. The renderer emphasises this band so the sentence and
	// the picture agree.
	Band int
	// Names lists the rows the finding can HONESTLY attribute. Empty when the
	// data cannot name anyone — never a guess.
	Names []string
	// High distinguishes the two dead-range ends: true = nothing up top
	// (sounds dull), false = nothing down low (sounds thin).
	High bool
}

// rowFacts is everything the finding engine needs about one drum row. Keeping
// it a plain value (rather than reaching into DrumRow / audio state) is what
// makes spectrumFindings a pure function the tests can drive directly.
type rowFacts struct {
	Name  string
	RMSDB float64
	// PeakDB is the loudest sample in the same analyzer window. Carried
	// alongside RMS because a percussive row's RMS collapses between hits
	// (measured: Dnb Kick -25.0 dB then -38.8 dB three frames later) while its
	// peak stays put, and judging audibility on RMS alone made the banner flip
	// findings several times a second.
	PeakDB    float64
	ClipCount int
	// FundHz is the row's sounding fundamental in Hz. Meaningful only when
	// Pitched is true.
	FundHz  float64
	Pitched bool
}

// Thresholds. These are judgment calls and are meant to be tuned BY EAR against
// real circuits — they are named and gathered here so that tuning is a one-file
// change rather than a hunt through the renderer.
const (
	// findingAudibleFloorDB — a row quieter than this is not meaningfully
	// participating in the mix, so it cannot be "fighting" anything.
	findingAudibleFloorDB = -45.0
	// findingPileUpDB — how far a band must stand above the mean of its
	// immediate neighbours before it counts as piled up. Self-relative, so it
	// carries no assumption about what the mix SHOULD look like.
	findingPileUpDB = 12.0
	// findingDeadFloorDB — at or below this a band holds essentially nothing.
	findingDeadFloorDB = -70.0
	// findingDeadMinBands — how many contiguous empty bands at an end before
	// it is worth mentioning. One empty band at the very top is normal.
	findingDeadMinBands = 3
)

// spectrumFindings returns the checkable observations about the current sound,
// most important first: clipping, then collision, then pile-up, then dead
// range. Returning an EMPTY slice is a valid and common answer — a tab that
// always finds a problem is noise.
//
// master may be nil or inactive; the row-only findings still hold.
func spectrumFindings(master *analyzer.ChannelMetrics, rows []rowFacts) []finding {
	var out []finding

	// ── Clipping ─────────────────────────────────────────────────────────
	// Needs no spectrum at all: the mixer already counted the samples that
	// exceeded full scale for this row.
	var clipped []string
	for _, r := range rows {
		if r.ClipCount > 0 {
			clipped = append(clipped, r.Name)
		}
	}
	if len(clipped) > 0 {
		sort.Strings(clipped)
		out = append(out, finding{Kind: findingClipping, Band: -1, Names: clipped})
	}

	// ── Collision ────────────────────────────────────────────────────────
	// Two or more audible, PITCHED rows sounding their fundamental inside one
	// ISO band are competing for the same space. Unpitched instruments (hats,
	// claps, shaker, cymbals) have no fundamental to collide with, so they are
	// never named — naming them would be a guess.
	byBand := map[int][]string{}
	for _, r := range rows {
		if !r.Pitched || r.FundHz <= 0 || !r.audible() {
			continue
		}
		if b := isoBandOf(r.FundHz); b >= 0 {
			byBand[b] = append(byBand[b], r.Name)
		}
	}
	collisions := make([]finding, 0, len(byBand))
	for b, names := range byBand {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		collisions = append(collisions, finding{Kind: findingCollision, Band: b, Names: names})
	}
	// Deterministic order: lowest band first, so the same mix always reports
	// the same sentence rather than whichever map key came up.
	sort.Slice(collisions, func(i, j int) bool { return collisions[i].Band < collisions[j].Band })
	out = append(out, collisions...)

	// ── Spectral findings ────────────────────────────────────────────────
	bands, ok := isoBandLevels(master)
	if !ok {
		return out
	}

	// Pile-up: a band far above the mean of its immediate neighbours.
	worst, worstExcess := -1, 0.0
	for b := range bands {
		sum, n := 0.0, 0
		for _, nb := range []int{b - 1, b + 1} {
			if nb >= 0 && nb < len(bands) {
				sum += bands[nb]
				n++
			}
		}
		if n == 0 {
			continue
		}
		if excess := bands[b] - sum/float64(n); excess >= findingPileUpDB && excess > worstExcess {
			worst, worstExcess = b, excess
		}
	}
	if worst >= 0 {
		out = append(out, finding{Kind: findingPileUp, Band: worst})
	}

	// Dead range: a run of empty bands at either end of the spectrum.
	high := 0
	for b := len(bands) - 1; b >= 0 && bands[b] <= findingDeadFloorDB; b-- {
		high++
	}
	low := 0
	for b := 0; b < len(bands) && bands[b] <= findingDeadFloorDB; b++ {
		low++
	}
	// A spectrum that is empty end-to-end is silence, not a dull mix.
	if low+high < len(bands) {
		if high >= findingDeadMinBands {
			out = append(out, finding{Kind: findingDeadRange, Band: len(bands) - high, High: true})
		}
		if low >= findingDeadMinBands {
			out = append(out, finding{Kind: findingDeadRange, Band: low - 1, High: false})
		}
	}
	return out
}

// audible reports whether the row is participating in the mix right now, on
// the steadier of the two level measures. A row silent on BOTH is genuinely
// not sounding.
func (r rowFacts) audible() bool {
	return r.RMSDB >= findingAudibleFloorDB || r.PeakDB >= findingAudibleFloorDB
}

// isoBandOf returns the ISO band index holding hz, or -1 when hz falls outside
// the table.
func isoBandOf(hz float64) int {
	for b := range isoBands {
		if hz >= isoBands[b][0] && hz < isoBands[b][1] {
			return b
		}
	}
	return -1
}

// isoBandLevels averages the master FFT into the 10 ISO bands, the same
// aggregation the bars themselves use. ok is false when there is no usable
// spectrum, in which case the caller reports no spectral findings at all
// rather than inventing them from an empty slice.
func isoBandLevels(ch *analyzer.ChannelMetrics) ([10]float64, bool) {
	var out [10]float64
	if ch == nil || !ch.Active || len(ch.FFTBins) == 0 || len(ch.FreqBins) != len(ch.FFTBins) {
		return out, false
	}
	for b := range isoBands {
		lo, hi := isoBands[b][0], isoBands[b][1]
		sum, n := 0.0, 0
		for i, hz := range ch.FreqBins {
			if hz >= lo && hz < hi {
				sum += ch.FFTBins[i]
				n++
			}
		}
		if n > 0 {
			out[b] = sum / float64(n)
		} else {
			out[b] = spectrumMinDB
		}
	}
	return out, true
}

// findingSentence renders one finding as the plain-English line shown in the
// Spectrum banner. It matches the voice already used by the tab legends
// ("Bass on the left, treble on the right") rather than analyser jargon.
//
// The frequency a sentence quotes is the ISO band's own LABEL ("62", "250",
// "4k") — the same string printed under the bars — so the sentence and the
// picture name the place identically. Quoting a computed centre frequency
// instead would have the banner say 63 Hz while the axis beneath it says 62.
func findingSentence(f finding) string {
	switch f.Kind {
	case findingClipping:
		return i18n.Tf(i18n.KeySpectrumFindClipping, joinNames(f.Names))
	case findingCollision:
		return i18n.Tf(i18n.KeySpectrumFindCollision, joinNames(f.Names), bandLabelAt(f.Band))
	case findingPileUp:
		return i18n.Tf(i18n.KeySpectrumFindPileUp, bandLabelAt(f.Band))
	case findingDeadRange:
		if f.High {
			return i18n.Tf(i18n.KeySpectrumFindDeadHigh, bandLabelAt(f.Band))
		}
		return i18n.Tf(i18n.KeySpectrumFindDeadLow, bandLabelAt(f.Band))
	}
	return ""
}

// bandLabelAt returns the Hz label printed under band b ("62", "4k"), or "" for
// a finding that is not band-scoped.
func bandLabelAt(b int) string {
	if b < 0 || b >= len(isoLabels) {
		return ""
	}
	return isoLabels[b]
}

// joinNames renders an instrument list the way a person would say it: "Kick",
// "Kick and Organ", "Kick, Organ and Bass".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " " + i18n.T(i18n.KeyAnd) + " " + names[1]
	}
	head := strings.Join(names[:len(names)-1], ", ")
	return head + " " + i18n.T(i18n.KeyAnd) + " " + names[len(names)-1]
}

// midiToHz converts a MIDI note number to its frequency. Kept next to the
// finding engine because an octave error here would silently place an
// instrument in the wrong band — mislabelling which rows are fighting — and
// this codebase has been bitten by octave errors in pitch detection before.
func midiToHz(midi int) float64 {
	return 440 * math.Pow(2, float64(midi-69)/12)
}

// spectrumRowFacts assembles the finding engine's per-row input from live
// state: the display name and level/clip counters from the analyzer, and the
// sounding fundamental from the instrument's own pitch info.
//
// The fundamental is taken at node pitch 0 — the instrument's BASE note, "where
// this instrument lives" — rather than any single node's pitch. A row whose
// nodes are pitched around still occupies its base register, and averaging live
// node pitches would make the banner flicker between bands as a pattern plays.
//
// Rows the analyzer has no metrics for are still reported (at silence) so a
// name is never dropped; rows whose instrument has no resolvable fundamental
// come back Pitched=false and are never named in a collision.
func (dv *DrumView) spectrumRowFacts(st *analyzer.State) []rowFacts {
	if dv == nil {
		return nil
	}
	levels := map[channel.ID]analyzer.InstrumentMetrics{}
	if st != nil {
		for _, im := range st.Instruments {
			levels[im.ID] = im
		}
	}
	out := make([]rowFacts, 0, len(dv.Rows))
	for _, r := range dv.Rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		f := rowFacts{Name: r.Name, RMSDB: spectrumMinDB, PeakDB: spectrumMinDB}
		if f.Name == "" {
			f.Name = string(r.Instrument)
		}
		if im, ok := levels[channel.Of(r.Instrument)]; ok {
			f.RMSDB, f.PeakDB, f.ClipCount = im.RMSDB, im.PeakDB, im.ClipCount
		}
		if offset, pitched := audio.InstrumentPitchInfo(r.Instrument); pitched {
			f.Pitched = true
			f.FundHz = midiToHz(nodePitchMIDI(0, offset))
		}
		out = append(out, f)
	}
	return out
}
