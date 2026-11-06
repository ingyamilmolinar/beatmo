package audiotap

import (
	"math"
	"strings"
	"testing"
)

// A digest framework that agrees with everything is worse than none: it
// converts "we have a test" into "we believe we have a test". These pin
// that each encoding actually DISCRIMINATES, and that the two digests
// differ in exactly the intended way.

func cap1(name string, v ...float64) *Snapshot {
	s := New("c", 48000)
	s.Floats(name, v...)
	return s
}

func TestIdenticalInputIsIdenticalBytes(t *testing.T) {
	a := cap1("x", 1.5, -2.25, 0)
	b := cap1("x", 1.5, -2.25, 0)
	if d := a.Diff(b); len(d) != 0 {
		t.Fatalf("identical input differed: %s", Report(d))
	}
	if a.Overall() != b.Overall() {
		t.Fatal("overall digest differs for identical input")
	}
}

func TestASingleChangedValueIsCaught(t *testing.T) {
	a := cap1("x", 1.5, -2.25, 0)
	b := cap1("x", 1.5, -2.25, 1e-5) // one quantum-visible change
	if d := a.Diff(b); len(d) == 0 {
		t.Fatal("a changed value produced an identical exact digest")
	}
	if d := a.DiffCrossPlatform(b); len(d) == 0 {
		t.Fatal("a changed value produced an identical cross-platform digest")
	}
}

// A length change must be reported AS a length change, not as "values
// differ" — that distinction is the whole diagnostic value of the package.
// It is also the shape the browser-only blank Spectrum took: 64 bins where
// 1024 were expected.
func TestALengthChangeIsReportedAsShape(t *testing.T) {
	a := cap1("shape.bins", 1, 2, 3, 4)
	b := cap1("shape.bins", 1, 2)
	d := a.Diff(b)
	if len(d) != 1 {
		t.Fatalf("want exactly one diff, got %v", d)
	}
	if !strings.Contains(d[0], "SHAPE changed") {
		t.Errorf("a length change must name the shape explicitly, got %q", d[0])
	}
	if !strings.Contains(Report(d), "a SHAPE section moved") {
		t.Errorf("Report must lead with the shape banner for a shape.* diff:\n%s", Report(d))
	}
}

// Length is folded into the hash, so a prefix cannot collide with the whole.
func TestPrefixDoesNotCollideWithWhole(t *testing.T) {
	a := cap1("x", 1, 2, 3)
	b := cap1("x", 1, 2, 3, 0)
	if a.Sections[0].Digest == b.Sections[0].Digest {
		t.Fatal("[1,2,3] and [1,2,3,0] hashed identically — the length prefix is not working")
	}
}

// The quantized digest must absorb the ~1 ULP that amd64-vs-wasm
// transcendental math legitimately produces, and nothing larger.
func TestQuantizedDigestAbsorbsOneULPButNotRealChange(t *testing.T) {
	const v = 0.034329453106454272 // the measured native value
	oneULP := math.Nextafter(v, 1)
	if oneULP == v {
		t.Fatal("Nextafter produced the same value")
	}
	a, b := cap1("x", v), cap1("x", oneULP)

	if d := a.Diff(b); len(d) == 0 {
		t.Error("the EXACT digest must still see a 1-ULP change — it is the within-platform contract")
	}
	if d := a.DiffCrossPlatform(b); len(d) != 0 {
		t.Errorf("the cross-platform digest must absorb 1 ULP (that is why it exists): %s", Report(d))
	}

	// One full quantum apart must NOT be absorbed.
	c := cap1("x", v+QuantumStep*2)
	if d := a.DiffCrossPlatform(c); len(d) == 0 {
		t.Errorf("the cross-platform digest absorbed a %g change — it is too coarse to be useful", QuantumStep*2)
	}
}

// Integer and boolean sections carry the SAME bytes in both digests, so for
// them the cross-platform comparison remains exact-bits. shape.* is built
// from integers precisely so it keeps that property.
func TestIntegerSectionsAreExactInBothDigests(t *testing.T) {
	s := New("c", 48000)
	s.Ints("shape.spectrum", 1024, 1024, 23437, 48000)
	s.Bools("flags", []bool{true, false, true})
	for _, sec := range s.Sections {
		if sec.Digest != sec.DigestQ {
			t.Errorf("%s: exact and quantized digests differ (%s vs %s) — an integer/bool section "+
				"must be bit-identical under both, or shape.* loses its cross-platform guarantee",
				sec.Name, short(sec.Digest), short(sec.DigestQ))
		}
	}
}

// NaN and -0 are normalized so two "no reading" values compare equal
// regardless of which arithmetic produced them.
func TestNaNAndNegativeZeroAreNormalized(t *testing.T) {
	a := cap1("x", math.NaN(), 0)
	b := cap1("x", math.Log(-1), math.Copysign(0, -1))
	if d := a.Diff(b); len(d) != 0 {
		t.Errorf("NaN/-0 not normalized: %s", Report(d))
	}
}

func TestDiffNamesMissingAndUnexpectedSections(t *testing.T) {
	a := New("c", 48000)
	a.Floats("only.in.a", 1)
	b := New("c", 48000)
	b.Floats("only.in.b", 1)
	d := a.Diff(b)
	joined := strings.Join(d, "\n")
	if !strings.Contains(joined, "only.in.a: UNEXPECTED") {
		t.Errorf("want UNEXPECTED for a section absent from the expectation, got:\n%s", joined)
	}
	if !strings.Contains(joined, "only.in.b: MISSING") {
		t.Errorf("want MISSING for an expected section the capture lacks, got:\n%s", joined)
	}
}

func TestSampleRateMismatchIsReported(t *testing.T) {
	a, b := New("c", 48000), New("c", 44100)
	if d := a.Diff(b); len(d) == 0 || !strings.Contains(d[0], "sample_rate") {
		t.Errorf("a sample-rate mismatch must be reported first, got %v", d)
	}
}
