package deploy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Reference recordings are never a test input: a test reading them passes, fails or skips by accident of one machine's
// disk. Scans for a wavref lookup without a scratch $BEATMO_WAV_REFS, a relative audio path, listing a directory for
// audio, a browser test reading audio off disk. Test audio is generated; corpus calibrations run as refs check.
func TestNoTestReadsReferenceAudio(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	walk := func(dir string, match func(string) bool, isGo bool) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !match(d.Name()) {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if rel == "src/go/internal/deploy/tests_read_no_audio_test.go" {
				return nil // this file names the patterns it forbids
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			offenders = append(offenders, scanTestSourceForAudioReads(rel, string(b), isGo)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	walk("src/go", func(n string) bool { return strings.HasSuffix(n, "_test.go") }, true)
	walk("src/js", func(n string) bool {
		return strings.HasSuffix(n, ".test.js") || strings.HasSuffix(n, ".test.mjs")
	}, false)
	if len(offenders) > 0 {
		t.Fatalf("tests must never read reference audio -- generate it (internal/wavgen, testwav) "+
			"or move a corpus calibration into `refs check`:\n  %s", strings.Join(offenders, "\n  "))
	}
}

var (
	audioExtRE   = `(?i:wav|wave|mp3|aiff?|flac|ogg|m4a|opus)`
	goLookupRE   = regexp.MustCompile(`\b(?:wavref\.)?(?:Path|Resolve|Dir|Root|SourcePath|SourcesDir)\(`)
	goWavrefRE   = regexp.MustCompile(`\bwavref\.(?:Path|Resolve|Dir|Root|SourcePath|SourcesDir)\(`)
	goIsolatedRE = regexp.MustCompile(`Setenv\((?:wavref\.)?EnvVar\b|Setenv\("BEATMO_WAV_REFS"`)
	relAudioRE   = regexp.MustCompile(`"\.\./[^"]*\.` + audioExtRE + `"`)
	listAudioRE  = regexp.MustCompile(`HasSuffix\([^\n]*"\.` + audioExtRE + `"\)|endsWith\(\s*"\.` + audioExtRE + `"\s*\)`)
	jsAssetsRE   = regexp.MustCompile(`["']assets["']\s*,\s*["']wav["']|["'][^"']*assets/wav`)
	jsReadRE     = regexp.MustCompile(`readFileSync\([^)]*\.` + audioExtRE + `\b`)
)

// scanTestSourceForAudioReads returns one line per way the test source at rel
// reads audio off disk.
func scanTestSourceForAudioReads(rel, src string, isGo bool) []string {
	var offenders []string
	code := stripLineComments(src)
	if isGo {
		lookup := goWavrefRE
		if strings.HasPrefix(rel, "src/go/internal/wavref/") {
			lookup = goLookupRE // the package's own tests call it unqualified
		}
		if lookup.MatchString(code) && !goIsolatedRE.MatchString(code) {
			offenders = append(offenders, rel+": resolves reference audio without pointing $BEATMO_WAV_REFS at t.TempDir()")
		}
	} else if jsAssetsRE.MatchString(code) || jsReadRE.MatchString(code) {
		offenders = append(offenders, rel+": reads audio files off disk")
	}
	if relAudioRE.MatchString(code) {
		offenders = append(offenders, rel+": relative path to an audio file")
	}
	// A browser test may inspect the .wav ENTRIES of a recording the app
	// itself produced; only listing a directory on DISK for audio is a read.
	if listAudioRE.MatchString(code) && (isGo || strings.Contains(code, "readdirSync")) {
		offenders = append(offenders, rel+": lists a directory for audio files")
	}
	return offenders
}

// So prose that NAMES a forbidden pattern is not mistaken for code doing it.
func stripLineComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 && !strings.Contains(line[:i], `"`) && !strings.Contains(line[:i], "'") && !strings.Contains(line[:i], "`") {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// The shapes that reached real audio before, plus those it must leave alone, so the walk cannot pass by matching nothing.
func TestAudioReadScanCatchesKnownOffenders(t *testing.T) {
	caught := []struct{ rel, src string }{
		{"src/go/x/kick_flux_test.go", `const path = "../../../../../36010__sandyrb__dnb-kick-003.wav"`},
		{"src/go/x/kick_metrics_test.go", `path, ok := wavref.Path("83768__zgump__kick-pack-0708.wav")`},
		{"src/go/x/fetch_test.go", `p, ok := wavref.Resolve("violin-A4")`},
		{"src/go/x/wavroot_test.go", `d, ok := wavref.Dir()`},
		{"src/go/x/songmatch_test.go", `if strings.HasSuffix(e.Name(), ".wav") {`},
		{"src/go/internal/wavref/x_test.go", `p, ok := Resolve("violin-A4")`},
		{"src/js/stress.browser.test.js", `const assetDir = path.join(repoRoot, "assets", "wav");`},
		{"src/js/x.browser.test.js", `const b = fs.readFileSync(path.join(dir, "kick.wav"));`},
		{"src/js/y.browser.test.js", "for (const e of fs.readdirSync(d)) if (e.name.endsWith(\".wav\")) n++;"},
	}
	for _, c := range caught {
		if got := scanTestSourceForAudioReads(c.rel, c.src, strings.HasSuffix(c.rel, ".go")); len(got) == 0 {
			t.Errorf("%s: scanner missed %q", c.rel, c.src)
		}
	}
	allowed := []struct{ rel, src string }{
		{"src/go/x/adopt_test.go", "t.Setenv(wavref.EnvVar, t.TempDir())\np, _ := wavref.Resolve(\"x\")"},
		{"src/go/x/list_test.go", `c, err := wavref.Catalogue()`},
		{"src/go/x/comment_test.go", `// it used to call wavref.Path("violin.wav")`},
		{"src/js/rec.browser.test.js", `if (name.endsWith(".wav")) wavs[name] = decodeWav(entries[name]);`},
		{"src/js/stress.browser.test.js", `// used to load the local, untracked assets/wav sample library`},
	}
	for _, c := range allowed {
		if got := scanTestSourceForAudioReads(c.rel, c.src, strings.HasSuffix(c.rel, ".go")); len(got) != 0 {
			t.Errorf("%s: false positive on %q: %v", c.rel, c.src, got)
		}
	}
}
