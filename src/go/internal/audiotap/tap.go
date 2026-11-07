// Package audiotap captures the data behind an audio tab as LAYERED byte-exact digests: shape.* (bin counts, widths,
// rates, lengths), analysis.* (what a tab reads), render.* (what it draws, through the renderer's own functions), so a
// failure names the layer that moved. Live analyser values are compared only on shape (Hann vs Blackman, different FFTs).
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

// Digest is sha256 over raw IEEE-754 bits, exact, for use WITHIN a platform; DigestQ quantises to QuantumStep for use
// ACROSS platforms, because amd64 (FMA) and wasm differ by one ULP downstream of a transcendental. Quantised ONCE here,
// never with "close enough" comparisons scattered through tests.
type Section struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`    // elements encoded (not bytes)
	Digest  string `json:"digest"`   // hex sha256, raw IEEE-754 bits
	DigestQ string `json:"digest_q"` // hex sha256, quantized to QuantumStep
}

// Nine orders above the ~1e-15 inter-platform noise (a value never straddles a boundary), four below anything a pixel
// can show.
const QuantumStep = 1e-6

// JSON-serialisable, so the browser hands one back to compare against the native golden.
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
		// Length-prefixed, so [a] and [a,0] never collide and a length-only change (the bin-count defect) moves the digest.
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

// Bits, never a formatted decimal, which would equate different values. NaN and -0 are normalised: equally "no reading".
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

// Between native and wasm, where raw bits legitimately differ by ~1 ULP through transcendental math.
func (s *Snapshot) DiffCrossPlatform(want *Snapshot) []string { return s.diff(want, true) }

// Names the LAYER that moved: the audio, the analysis shape, or only the drawing.
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
			// A length change means the data's SHAPE moved, which silently redraws everything downstream.
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
