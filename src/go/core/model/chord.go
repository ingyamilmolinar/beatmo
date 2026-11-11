package model

import (
	"math"
	"sort"
)

// chord.go — the chord node's spec and the voice stack ("z-axis") derived
// from it (docs/superpowers/specs/2026-10-09-chord-nodes-design.md).
//
// A node stores WHAT the user picked (ChordSpec); NodeLayers is the single
// place that turns it into simultaneous voices. Everything below the model —
// the audio lane, legacy dispatch, offline render — consumes only []Layer,
// so the chord is just the first producer of a voice stack.

// ChordSpec is a chord node's quality and voicing. The root is the node's
// own Pitch, so ± and group transposition move the whole chord. The spec is
// independent of the node type: it is read only while the type is
// NodeTypeChord but kept on every type, so collapsing a chord to a single
// note and back restores it.
type ChordSpec struct {
	Quality   string // key into ChordQualities; "" = no spec
	Inversion int    // 0 = root position … len(intervals)-1
	Spread    int    // ChordSpreadClose | ChordSpreadDrop2 | ChordSpreadWide
}

// IsZero reports an empty spec (nothing picked yet).
func (c ChordSpec) IsZero() bool { return c == ChordSpec{} }

const (
	ChordSpreadClose = 0 // stacked within the inversion
	ChordSpreadDrop2 = 1 // second-highest voice down an octave
	ChordSpreadWide  = 2 // drop-2, plus the original lowest voice down an octave
	ChordSpreadCount = 3
)

// ChordDefaultQuality seeds a chord node that has no spec yet.
const ChordDefaultQuality = "maj"

// ChordQuality is one column of the chord matrix.
type ChordQuality struct {
	ID        string // stable id written to the project file
	Suffix    string // appended to the root name ("m7", "°", "")
	Page      string // one of ChordPages
	Intervals []int  // semitones from the root, ascending, starting at 0
}

// ChordPages are the matrix's quality pages, five qualities each so a page
// fits a phone at the touch minimum.
var ChordPages = []string{"Triads", "7ths", "Ext", "Sus"}

// ChordQualities is the one quality table (ordered by page, then column).
var ChordQualities = []ChordQuality{
	{ID: "maj", Suffix: "", Page: "Triads", Intervals: []int{0, 4, 7}},
	{ID: "m", Suffix: "m", Page: "Triads", Intervals: []int{0, 3, 7}},
	{ID: "dim", Suffix: "°", Page: "Triads", Intervals: []int{0, 3, 6}},
	{ID: "aug", Suffix: "+", Page: "Triads", Intervals: []int{0, 4, 8}},
	{ID: "5", Suffix: "5", Page: "Triads", Intervals: []int{0, 7}},

	{ID: "7", Suffix: "7", Page: "7ths", Intervals: []int{0, 4, 7, 10}},
	{ID: "maj7", Suffix: "maj7", Page: "7ths", Intervals: []int{0, 4, 7, 11}},
	{ID: "m7", Suffix: "m7", Page: "7ths", Intervals: []int{0, 3, 7, 10}},
	{ID: "m7b5", Suffix: "ø7", Page: "7ths", Intervals: []int{0, 3, 6, 10}},
	{ID: "dim7", Suffix: "°7", Page: "7ths", Intervals: []int{0, 3, 6, 9}},

	{ID: "6", Suffix: "6", Page: "Ext", Intervals: []int{0, 4, 7, 9}},
	{ID: "m6", Suffix: "m6", Page: "Ext", Intervals: []int{0, 3, 7, 9}},
	{ID: "9", Suffix: "9", Page: "Ext", Intervals: []int{0, 4, 7, 10, 14}},
	{ID: "maj9", Suffix: "maj9", Page: "Ext", Intervals: []int{0, 4, 7, 11, 14}},
	{ID: "m9", Suffix: "m9", Page: "Ext", Intervals: []int{0, 3, 7, 10, 14}},

	{ID: "sus2", Suffix: "sus2", Page: "Sus", Intervals: []int{0, 2, 7}},
	{ID: "sus4", Suffix: "sus4", Page: "Sus", Intervals: []int{0, 5, 7}},
	{ID: "7sus4", Suffix: "7sus4", Page: "Sus", Intervals: []int{0, 5, 7, 10}},
	{ID: "add9", Suffix: "add9", Page: "Sus", Intervals: []int{0, 4, 7, 14}},
	{ID: "madd9", Suffix: "m(add9)", Page: "Sus", Intervals: []int{0, 3, 7, 14}},
}

// ChordQualityByID looks a quality up by its id.
func ChordQualityByID(id string) (ChordQuality, bool) {
	for _, q := range ChordQualities {
		if q.ID == id {
			return q, true
		}
	}
	return ChordQuality{}, false
}

// ChordQualitiesOnPage returns one page's qualities in column order.
func ChordQualitiesOnPage(page string) []ChordQuality {
	var out []ChordQuality
	for _, q := range ChordQualities {
		if q.Page == page {
			out = append(out, q)
		}
	}
	return out
}

// ChordPageOf returns the page index holding a quality (0 when unknown).
func ChordPageOf(id string) int {
	q, ok := ChordQualityByID(id)
	if !ok {
		return 0
	}
	for i, p := range ChordPages {
		if p == q.Page {
			return i
		}
	}
	return 0
}

// NormalizeChord clamps a spec into range. An empty quality stays empty; an
// unknown quality yields the zero spec.
func NormalizeChord(c ChordSpec) ChordSpec {
	if c.Quality == "" {
		return ChordSpec{}
	}
	q, ok := ChordQualityByID(c.Quality)
	if !ok {
		return ChordSpec{}
	}
	c.Inversion = clampInt(c.Inversion, 0, len(q.Intervals)-1)
	c.Spread = clampInt(c.Spread, 0, ChordSpreadCount-1)
	return c
}

// ChordSpecValid reports whether a spec is already in range (no clamping
// needed) — the import validator rejects anything else.
func ChordSpecValid(c ChordSpec) bool {
	if c.Quality == "" {
		return c.IsZero()
	}
	return NormalizeChord(c) == c
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Layer is one voice of a node's stack: a semitone offset from the node's
// Pitch and a gain multiplier on its volume.
type Layer struct {
	Semis, Gain float64
}

// effectiveChord is the spec a chord node actually plays (an empty spec is
// the default major triad).
func effectiveChord(c ChordSpec) (ChordSpec, ChordQuality) {
	c = NormalizeChord(c)
	if c.Quality == "" {
		c = ChordSpec{Quality: ChordDefaultQuality}
	}
	q, _ := ChordQualityByID(c.Quality)
	return c, q
}

// NodeLayers returns the node's voice stack. nil means a single voice at the
// node's own pitch and volume (every non-chord node — callers keep today's
// path untouched for it).
func NodeLayers(t NodeType, p NodeParams) []Layer {
	if t != NodeTypeChord {
		return nil
	}
	c, q := effectiveChord(p.Chord)
	v := invertedIntervals(q, c.Inversion)
	if c.Spread >= ChordSpreadDrop2 {
		low := v[0]
		if len(v) == 2 {
			v[0] -= 12
		} else {
			v[len(v)-2] -= 12
			if c.Spread == ChordSpreadWide {
				for i := range v {
					if v[i] == low {
						v[i] -= 12
						break
					}
				}
			}
		}
		sort.Ints(v)
	}
	gain := 1 / math.Sqrt(float64(len(v)))
	out := make([]Layer, len(v))
	for i, s := range v {
		out[i] = Layer{Semis: float64(s), Gain: gain}
	}
	return out
}

// invertedIntervals: the lowest k voices moved up an octave, ascending.
func invertedIntervals(q ChordQuality, k int) []int {
	v := make([]int, len(q.Intervals))
	copy(v, q.Intervals)
	for i := 0; i < k && i < len(v); i++ {
		v[i] += 12
	}
	sort.Ints(v)
	return v
}

var pitchClassNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// PitchClassName names a pitch class with sharps (any int; wraps mod 12).
func PitchClassName(pc int) string { return pitchClassNames[((pc%12)+12)%12] }

// ChordName names a chord on a root ("C#m7"), with "/<bass>" when the
// inversion puts a non-root in the bass ("Cmaj7/E"). Spread never changes
// the name.
func ChordName(rootMIDI int, c ChordSpec) string {
	c, q := effectiveChord(c)
	name := PitchClassName(rootMIDI) + q.Suffix
	if c.Inversion > 0 {
		name += "/" + PitchClassName(rootMIDI+q.Intervals[c.Inversion])
	}
	return name
}

// Fires reports whether a node of this type triggers a sound on the
// timeline. The timeline cannot tell a chord from a regular node: one cell,
// one trigger; the voice stack only matters where a note becomes sound.
func (t NodeType) Fires() bool { return t == NodeTypeRegular || t == NodeTypeChord }
