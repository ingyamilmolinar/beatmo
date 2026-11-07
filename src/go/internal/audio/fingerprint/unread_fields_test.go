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

// A computed number nothing reads is a cost, not a measurement, and the failure is silent (it has unit tests and shows
// up nowhere); the package produced it repeatedly. Structural: every exported field of a metrics struct must be read
// outside the file computing it.

// Adding an analyser means saying who will read it.
var metricStructs = []string{
	"Fingerprint", "EnvelopeSegments", "Tracks", "TrackStats",
	"SnareMetrics", "PianoMetrics", "WindAttack", "SpectralEnvelope",
	"NoiseProfile", "HarmonicTrajectory", "PartialRatios",
}

// Each with its reason: a decision on the record; removing the field is always the alternative.
var unreadAllowed = map[string]string{
	// A curated subset of the cross-product is registered by design.
	"TrackStats.Mean":  "uniform statistic set; registry_tracks.go registers a curated subset by design",
	"TrackStats.Std":   "uniform statistic set; see TrackStats.Mean",
	"TrackStats.P10":   "uniform statistic set; see TrackStats.Mean",
	"TrackStats.Range": "uniform statistic set; see TrackStats.Mean",
	"Tracks.Rolloff":   "frame-wise rolloff kept parallel to Centroid so the two can be correlated by a future descriptor; no row reads it yet",

	// distance.go's composite scalar is what the per-axis registry replaces; its inputs are not re-registered.
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

// Descriptor Docs name fields in prose, and a mention must not count as a read.
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
			// A Doc built with + is a string, not a comment, and mentions fields the same way.
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

// The key consumers live outside the package (cmd/refs prints PartialRatios.Ratios).
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
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

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

// Once a field gains a consumer, its excuse is deleted.
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
