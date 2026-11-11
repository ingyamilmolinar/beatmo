// Package audiotap captures the data behind an audio tab as a LAYERED,
// byte-exact digest, so "the same circuit renders the same thing" is a
// mechanical check rather than a screenshot someone has to look at.
//
// # Why layered
//
// The defect this package exists to catch (2026-09-19, Spectrum tab) had a
// correct audio path, a correct renderer, and an empty plot. The browser's
// analyser bridge delivered 64 spectrum bins spanning 0..Nyquist — 375 Hz
// apart — while the narrowest display band is 24 Hz wide, so the four
// lowest bands contained no bin at all and reported the reference floor.
// Every byte of audio was right. Every line of drawing code was right. The
// SHAPE of the data between them was wrong, and nothing in the repo
// compared shapes.
//
// A single end-to-end digest would have gone red and said nothing useful.
// So a Snapshot is a list of independently-digested SECTIONS, ordered from
// the audio outwards:
//
//	shape.*     — bin counts, bin width, sample rate, buffer lengths.
//	              Cross-platform comparable, and the layer that broke.
//	analysis.*  — the analyzer input a tab reads (FFTBins, Waveform, levels).
//	render.*    — the numbers the tab will actually draw, derived through
//	              the SAME functions the renderer calls.
//
// Diff reports which sections moved, so a failure reads "shape.spectrum
// changed, analysis and render followed" rather than "the digest changed".
//
// # Why byte-exact is legitimate here
//
// Encoding is IEEE-754 bit patterns, little-endian, length-prefixed — no
// rounding, no formatting, no map iteration. Identical inputs through
// identical Go code produce identical bytes, natively and under GOOS=js
// (the property internal/ui's cross-platform parity suite already relies
// on). That makes a native golden a legitimate expectation for the browser
// to meet.
//
// What is NOT comparable across platforms, and must not be faked: the
// values a LIVE analyser reports. Go's observer uses a Hann window and its
// own FFT; a WebAudio AnalyserNode uses Blackman and the browser's. Same
// audio, different last bits — by design. Cross-platform assertions
// therefore run over shape and over render-from-canonical-input; live
// values are compared only on the shape layer, which is exactly where the
// defect lived.
package audiotap

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Section is one independently-digested slice of a capture.
//
// TWO digests, because there are two different questions and only one
// answer is honest for each:
//
//	Digest  — sha256 over raw IEEE-754 bits. Exact. Use it WITHIN a
//	          platform: determinism, and the committed golden. Any change
//	          whatsoever moves it.
//	DigestQ — sha256 over the same values quantized to QuantumStep. Use it
//	          ACROSS platforms.
//
// The cross-platform digest exists because raw bits are NOT reproducible
// between amd64 and wasm for anything downstream of a transcendental.
// Measured on this repo's own fixture: math.Exp(-1000/90) returns
// 0.034329453106454272 natively and 0.03432945310645428 under GOOS=js —
// one ULP apart, because amd64's implementation uses FMA and wasm's does
// not. It propagates to ~8e-15 dB.
//
// That is not a defect and must not be papered over with "close enough"
// float comparisons scattered through the tests. It is a known, bounded,
// one-ULP property of the platform, so the canonical form quantizes it
// away ONCE, here, at a step chosen to sit far above the noise and far
// below anything observable — and the comparison stays byte-for-byte.
type Section struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`    // elements encoded (not bytes)
	Digest  string `json:"digest"`   // hex sha256, raw IEEE-754 bits
	DigestQ string `json:"digest_q"` // hex sha256, quantized to QuantumStep
}

// QuantumStep is the grid the cross-platform digest snaps values to.
//
// Nine orders of magnitude above the ~1e-15 inter-platform noise, so a
// value can effectively never straddle a boundary and hash differently on
// the two platforms. Four orders below anything renderable: a 42 dB window
// across a ~500 px plot resolves ~0.08 dB, and amplitudes live in [-1,1].
// A real difference this test should catch is thousands of quanta wide; the
// noise it must ignore is a millionth of one.
const QuantumStep = 1e-6

// Snapshot is an ordered set of sections plus the identity of what was
// captured. JSON-serializable so the browser can hand one back to a test
// and it can be compared against a committed native golden.
type Snapshot struct {
	Circuit    string    `json:"circuit"`
	SampleRate int       `json:"sample_rate"`
	Sections   []Section `json:"sections"`
}

// New starts a capture.
func New(circuit string, sampleRate int) *Snapshot {
	return &Snapshot{Circuit: circuit, SampleRate: sampleRate}
}

// add hashes one section twice: exactly, and on the quantized grid.
// `write` is called once per digest with the encoder to use.
func (s *Snapshot) add(name string, count int, write func(w func([]byte), quantized bool)) {
	digest := func(quantized bool) string {
		h := sha256.New()
		// Length-prefix so [a] and [a,0] cannot collide, and so a section
		// that changes ONLY in length — the bin-count defect — always moves
		// the digest even if every surviving value is unchanged.
		var n [8]byte
		binary.LittleEndian.PutUint64(n[:], uint64(count))
		h.Write(n[:])
		write(func(b []byte) { h.Write(b) }, quantized)
		return hex.EncodeToString(h.Sum(nil))
	}
	s.Sections = append(s.Sections, Section{
		Name:    name,
		Count:   count,
		Digest:  digest(false),
		DigestQ: digest(true),
	})
}

// Floats adds a float64 slice as raw IEEE-754 bits.
//
// Bits, not a formatted decimal: a %.6f would quietly equate two values
// that differ, which is the opposite of what a byte-for-byte check is for.
// NaN is normalized to a single quiet-NaN pattern so two NaNs from
// different arithmetic compare equal (they are equally "no reading"), and
// -0 is normalized to +0 for the same reason.
func (s *Snapshot) Floats(name string, v ...float64) {
	s.add(name, len(v), func(w func([]byte), quantized bool) {
		var b [8]byte
		for _, f := range v {
			if quantized {
				binary.LittleEndian.PutUint64(b[:], uint64(quantize(f)))
			} else {
				binary.LittleEndian.PutUint64(b[:], canonicalBits(f))
			}
			w(b[:])
		}
	})
}

// quantize snaps a value onto the QuantumStep grid as a signed integer.
// Non-finite values collapse to a single sentinel so two NaNs — or two
// infinities of the same sign — compare equal.
func quantize(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return math.MinInt64
	case math.IsInf(f, 1):
		return math.MaxInt64
	case math.IsInf(f, -1):
		return math.MinInt64 + 1
	}
	return int64(math.Round(f / QuantumStep))
}

// Float32s adds a float32 slice (PCM) as raw IEEE-754 bits.
func (s *Snapshot) Float32s(name string, v []float32) {
	s.add(name, len(v), func(w func([]byte), quantized bool) {
		var b [8]byte
		for _, f := range v {
			if quantized {
				binary.LittleEndian.PutUint64(b[:], uint64(quantize(float64(f))))
			} else {
				binary.LittleEndian.PutUint64(b[:], canonicalBits(float64(f)))
			}
			w(b[:])
		}
	})
}

// Ints adds integers — the shape layer's currency.
func (s *Snapshot) Ints(name string, v ...int) {
	// Integers are exact everywhere, so both digests see the same bytes.
	s.add(name, len(v), func(w func([]byte), _ bool) {
		var b [8]byte
		for _, i := range v {
			binary.LittleEndian.PutUint64(b[:], uint64(int64(i)))
			w(b[:])
		}
	})
}

// Bools adds flags (per-band "was this measured", per-channel Active).
func (s *Snapshot) Bools(name string, v []bool) {
	s.add(name, len(v), func(w func([]byte), _ bool) {
		b := []byte{0}
		for _, t := range v {
			if t {
				b[0] = 1
			} else {
				b[0] = 0
			}
			w(b)
		}
	})
}

// Strings adds text the UI shows (a headline sentence, axis labels).
func (s *Snapshot) Strings(name string, v ...string) {
	s.add(name, len(v), func(w func([]byte), _ bool) {
		var n [8]byte
		for _, str := range v {
			binary.LittleEndian.PutUint64(n[:], uint64(len(str)))
			w(n[:])
			w([]byte(str))
		}
	})
}

// canonicalBits normalizes the two float patterns that are "equal" for our
// purposes but differ bitwise.
func canonicalBits(f float64) uint64 {
	if math.IsNaN(f) {
		return 0x7FF8000000000000 // one quiet NaN
	}
	if f == 0 {
		return 0 // collapse -0 onto +0
	}
	return math.Float64bits(f)
}

// Overall is the exact digest of every section in order — the one-line
// answer for same-platform comparison.
func (s *Snapshot) Overall() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|", s.Circuit, s.SampleRate)
	for _, sec := range s.Sections {
		fmt.Fprintf(h, "%s=%d:%s;", sec.Name, sec.Count, sec.Digest)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Diff compares exactly (raw IEEE-754 bits). Use within one platform.
func (s *Snapshot) Diff(want *Snapshot) []string { return s.diff(want, false) }

// DiffCrossPlatform compares on the QuantumStep grid. Use between native
// and wasm, where raw bits legitimately differ by ~1 ULP through the
// transcendental math (see Section).
func (s *Snapshot) DiffCrossPlatform(want *Snapshot) []string { return s.diff(want, true) }

// diff reports, per section, how this capture differs from want. It is the
// point of the whole package: the report names the LAYER, so a reader
// learns whether the audio moved, the analysis shape moved, or only the
// drawing did.
func (s *Snapshot) diff(want *Snapshot, quantized bool) []string {
	var out []string
	if want == nil {
		return []string{"no expected snapshot to compare against"}
	}
	if s.SampleRate != want.SampleRate {
		out = append(out, fmt.Sprintf("sample_rate: got %d, want %d", s.SampleRate, want.SampleRate))
	}
	index := func(snap *Snapshot) map[string]Section {
		m := make(map[string]Section, len(snap.Sections))
		for _, sec := range snap.Sections {
			m[sec.Name] = sec
		}
		return m
	}
	got, exp := index(s), index(want)
	names := map[string]bool{}
	for n := range got {
		names[n] = true
	}
	for n := range exp {
		names[n] = true
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	for _, n := range ordered {
		g, gok := got[n]
		e, eok := exp[n]
		switch {
		case !gok:
			out = append(out, fmt.Sprintf("%s: MISSING (expected %d elements)", n, e.Count))
		case !eok:
			out = append(out, fmt.Sprintf("%s: UNEXPECTED (%d elements)", n, g.Count))
		case g.Count != e.Count:
			// Called out separately because it is the failure this package
			// was built for: a length change means the SHAPE of the data
			// moved, which silently redraws everything downstream.
			out = append(out, fmt.Sprintf("%s: length %d, want %d — the data's SHAPE changed, "+
				"not just its values", n, g.Count, e.Count))
		default:
			gd, ed := g.Digest, e.Digest
			if quantized {
				gd, ed = g.DigestQ, e.DigestQ
			}
			if gd != ed {
				out = append(out, fmt.Sprintf("%s: %d elements, values differ (%s != %s)",
					n, g.Count, short(gd), short(ed)))
			}
		}
	}
	return out
}

// short truncates a hex digest for a readable failure line.
func short(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

// Report renders a Diff for a test failure message, grouped by layer so the
// first line already says where to look.
func Report(diffs []string) string {
	if len(diffs) == 0 {
		return "identical"
	}
	var b strings.Builder
	layer := func(d string) string {
		if i := strings.Index(d, "."); i > 0 {
			return d[:i]
		}
		return "other"
	}
	seen := map[string]bool{}
	var order []string
	for _, d := range diffs {
		l := layer(d)
		if !seen[l] {
			seen[l] = true
			order = append(order, l)
		}
	}
	fmt.Fprintf(&b, "%d section(s) differ, in layer(s): %s\n", len(diffs), strings.Join(order, ", "))
	if seen["shape"] {
		b.WriteString("  ** a SHAPE section moved — the analyser is delivering differently-sized " +
			"data, which changes every derived value downstream. Fix that first; the rest is fallout.\n")
	}
	for _, d := range diffs {
		fmt.Fprintf(&b, "  - %s\n", d)
	}
	return b.String()
}
