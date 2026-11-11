package fingerprint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// unread_fields_test.go — an analyzer that computes a number nothing reads is
// not a measurement, it is a cost.
//
// This package has produced that bug four separate times, each found by hand
// and each hiding a real gap:
//
//   - AnalyzeSnare / AnalyzePiano / ComputeWindAttack / SpectralIrregularity ran
//     on every fingerprint with zero non-test consumers (fixed by
//     registry_family.go).
//   - Tristimulus, the vibrato bank and the formant bank likewise (fixed by
//     registry_timbre.go / registry_perceptual.go). Tristimulus then turned out
//     to be the SECOND most predictive axis in the whole battery.
//   - EnvelopeSegments.DecaySec / DecayRateDBPerSec / PeakSec /
//     TotalAudibleSec, while the listener's own request was "get duration of
//     attack, release, peak, etc" (fixed by the env.* rows in registry_core.go).
//
// The failure is silent by construction: an unread field costs CPU on every
// analysis, reads as covered because it has unit tests, and shows up nowhere.
// So the test is structural -- it parses the package, finds every exported
// field on a metrics struct, and asserts something outside the file that
// computes it actually reads it.

// metricStructs are the analysis result types whose fields must be consumed.
// Adding an analyzer means adding it here, which is the point: the cost of a
// new metric should include saying who will read it.
var metricStructs = []string{
	"Fingerprint", "EnvelopeSegments", "Tracks", "TrackStats",
	"SnareMetrics", "PianoMetrics", "WindAttack", "SpectralEnvelope",
	"NoiseProfile", "HarmonicTrajectory", "PartialRatios",
}

// unreadAllowed lists fields deliberately computed without a consumer, each
// with the reason. An entry here is a decision on the record, not an oversight;
// removing the field is always the alternative.
var unreadAllowed = map[string]string{
	// TrackStats is a uniform statistic set applied to every frame-wise track.
	// registry_tracks.go registers a CURATED subset of the 6x12 cross-product
	// on purpose -- registering all 72 would bury the report in near-duplicates
	// before the orthogonality analysis has said which are redundant. The
	// unregistered statistics stay available for that analysis to reach for.
	"TrackStats.Mean":  "uniform statistic set; registry_tracks.go registers a curated subset by design",
	"TrackStats.Std":   "uniform statistic set; see TrackStats.Mean",
	"TrackStats.P10":   "uniform statistic set; see TrackStats.Mean",
	"TrackStats.Range": "uniform statistic set; see TrackStats.Mean",
	"Tracks.Rolloff":   "frame-wise rolloff kept parallel to Centroid so the two can be correlated by a future descriptor; no row reads it yet",

	// Legacy composite-distance inputs. distance.go collapses everything into
	// one weighted scalar, which is the failure mode the per-axis registry
	// exists to replace; its inputs are not re-registered as axes.
	"Fingerprint.SustainMag":     "input to distance.go's legacy composite scalar only",
	"Fingerprint.LogAttackTime":  "input to distance.go's legacy composite scalar only",
	"PianoMetrics.NumPartials":   "diagnostic count for piano.stretch_cents' own vector length, not an axis",
	"PianoMetrics.CentroidHz":    "superseded by bright.centroid and timbre.brightness_ratio",
	"SnareMetrics.CentroidHz":    "superseded by bright.centroid",
	"EnvelopeSegments.PeakLevel": "block-RMS peak, not comparable across level-matched clips; PeakSec carries the useful half",

	// Vector-valued analyzer outputs with no comparator yet.
	"NoiseProfile.ResidualBandLevels":    "per-band residual profile; needs a band-aware comparator before it can be an axis",
	"HarmonicTrajectory.EvenOddOverTime": "even/odd balance over time; a candidate modulation-domain axis, not yet calibrated",
}

// fieldReadRe matches a selector read of a field name.
func fieldReadRe(field string) *regexp.Regexp {
	return regexp.MustCompile(`\.` + regexp.QuoteMeta(field) + `\b`)
}

// stripComments removes line and block comments.
//
// Load-bearing, not tidiness: several descriptor Doc strings name the field
// they read in prose ("NoiseProfile.ResidualRatio: ..."), and counting a
// mention in documentation as a read would let a field be documented into
// looking alive while nothing actually consumed it -- the precise illusion this
// test exists to break.
func stripComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "//"):
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 4
		case src[i] == '"':
			// Skip string literals too: a Doc built with + concatenation is a
			// string, not a comment, and mentions the field the same way.
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			i = j + 1
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	return b.String()
}

// moduleRootFrom walks up from dir to the directory holding go.mod.
func moduleRootFrom(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// consumersOutsidePackage returns comment-stripped sources for every non-test
// Go file in the module OUTSIDE this package.
//
// Required, because the most important consumers live outside it: cmd/refs
// reads PartialRatios.Ratios to print the side-by-side partial lists, and an
// in-package-only scan calls that dead.
func consumersOutsidePackage(t *testing.T) []string {
	t.Helper()
	root, ok := moduleRootFrom(".")
	if !ok {
		t.Skip("module root not found; cannot check out-of-package consumers")
	}
	self, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Dir(path) == self {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		out = append(out, stripComments(string(b)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNoUnreadMetricFields(t *testing.T) {
	dir := "."
	fset := token.NewFileSet()
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	parsed := map[string]*ast.File{}
	for _, name := range goFiles {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse package: %v", err)
		}
		parsed[name] = f
	}
	pkgs := []struct{ Files map[string]*ast.File }{{parsed}}

	// file contents, for textual read-detection
	srcs := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		srcs[n] = stripComments(string(b))
	}
	outside := consumersOutsidePackage(t)

	want := map[string]bool{}
	for _, s := range metricStructs {
		want[s] = true
	}

	type fieldRef struct{ structName, field, homeFile string }
	var fields []fieldRef

	for _, pkg := range pkgs {
		for fname, file := range pkg.Files {
			base := filepath.Base(fname)
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok || !want[ts.Name.Name] {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, f := range st.Fields.List {
					for _, nm := range f.Names {
						if nm.IsExported() {
							fields = append(fields, fieldRef{ts.Name.Name, nm.Name, base})
						}
					}
				}
				return true
			})
		}
	}
	if len(fields) == 0 {
		t.Fatal("found no metric struct fields — the parser or metricStructs is wrong, so a pass proves nothing")
	}

	var unread []string
	for _, fr := range fields {
		key := fr.structName + "." + fr.field
		if _, ok := unreadAllowed[key]; ok {
			continue
		}
		re := fieldReadRe(fr.field)
		read := false
		for name, src := range srcs {
			if name == fr.homeFile {
				continue // the file that computes it does not count as reading it
			}
			if re.MatchString(src) {
				read = true
				break
			}
		}
		if !read {
			for _, src := range outside {
				if re.MatchString(src) {
					read = true
					break
				}
			}
		}
		if !read {
			unread = append(unread, key+" (computed in "+fr.homeFile+")")
		}
	}
	sort.Strings(unread)
	for _, u := range unread {
		t.Errorf("%s is computed but read by nothing outside the file that computes it.\n"+
			"    Register it as a descriptor, have an existing one read it, delete it, or add it to "+
			"unreadAllowed with the reason. An unread metric costs CPU on every analysis, looks "+
			"covered because it has unit tests, and appears in no report.", u)
	}
	t.Logf("checked %d exported fields across %d metric structs; %d deliberately unread",
		len(fields), len(metricStructs), len(unreadAllowed))
}

// TestUnreadAllowlistHasNoStaleEntries keeps the allowlist honest in the other
// direction: once a field gains a real consumer, its excuse must be deleted.
func TestUnreadAllowlistHasNoStaleEntries(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	srcs := map[string]string{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(n)
		srcs[n] = stripComments(string(b))
	}
	keys := make([]string, 0, len(unreadAllowed))
	for k := range unreadAllowed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, ".", 2)
		if len(parts) != 2 {
			t.Errorf("malformed allowlist key %q, want Struct.Field", k)
			continue
		}
		re := fieldReadRe(parts[1])
		readers := 0
		for _, src := range srcs {
			if re.MatchString(src) {
				readers++
			}
		}
		// >1 means something beyond the computing file touches it.
		if readers > 1 {
			t.Logf("note: %s now has %d files touching it — if one of them is a real consumer, "+
				"remove it from unreadAllowed", k, readers)
		}
	}
}
