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

// A finding states only what is CHECKABLE from the data, never taste: a drum machine has no correct spectrum.
// Kinds: clipping, collision, pile-up (self-relative), dead range. No per-row FFT: only master and one channel carry spectra.
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
	// -1 when not band-scoped; the renderer emphasises this band so the sentence and picture agree.
	Band int
	// Only rows the data can HONESTLY name; empty rather than a guess.
	Names []string
	// true = nothing up top (dull), false = nothing down low (thin).
	High bool
}

// A plain value so spectrumFindings stays a pure function tests drive directly.
type rowFacts struct {
	Name  string
	RMSDB float64
	// A percussive row's RMS collapses between hits while its peak holds; judging on RMS alone made the banner
	// flip findings several times a second.
	PeakDB    float64
	ClipCount int
	// Meaningful only when Pitched.
	FundHz  float64
	Pitched bool
}

// Judgment calls, tuned by ear against real circuits; gathered here so tuning is a one-file change.
const (
	// A row quieter than this is not in the mix, so it cannot be "fighting" anything.
	findingAudibleFloorDB = -45.0
	// Above the mean of the immediate neighbours: self-relative, so no assumption about how a mix should look.
	findingPileUpDB = 12.0
	// findingDeadFloorDB — at or below this a band holds essentially nothing.
	findingDeadFloorDB = -70.0
	// One empty band at the very top is normal.
	findingDeadMinBands = 3
)

// Most important first. An empty slice is a valid, common answer: a tab that always finds a problem is noise.
// master may be nil; the row-only findings still hold.
func spectrumFindings(master *analyzer.ChannelMetrics, rows []rowFacts) []finding {
	var out []finding

	// Needs no spectrum: the mixer already counted this row's samples over full scale.
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

	// Pitched rows only: unpitched instruments have no fundamental to collide with, and naming them would be a guess.
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
	// Lowest band first so the same mix always reports the same sentence.
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

// A row silent on BOTH level measures is genuinely not sounding.
func (r rowFacts) audible() bool {
	return r.RMSDB >= findingAudibleFloorDB || r.PeakDB >= findingAudibleFloorDB
}

func isoBandOf(hz float64) int {
	for b := range isoBands {
		if hz >= isoBands[b][0] && hz < isoBands[b][1] {
			return b
		}
	}
	return -1
}

// The bars' own aggregation; !ok means no spectral findings at all rather than findings from an empty slice.
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

// Quotes the band's own label ("62", "4k"), the string under the bars, so the sentence and the axis never
// disagree (a computed centre would say 63 above a 62).
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

func bandLabelAt(b int) string {
	if b < 0 || b >= len(isoLabels) {
		return ""
	}
	return isoLabels[b]
}

// "Kick", "Kick and Organ", "Kick, Organ and Bass".
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

// An octave error here would silently place an instrument in the wrong band and misname who is fighting.
func midiToHz(midi int) float64 {
	return 440 * math.Pow(2, float64(midi-69)/12)
}

// The fundamental is the instrument's BASE note (node pitch 0): averaging live node pitches would make the
// banner flicker between bands as a pattern plays. Rows without metrics report silence, so no name is dropped.
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
