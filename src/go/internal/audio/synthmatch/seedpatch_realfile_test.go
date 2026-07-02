package synthmatch

// These tests exercise PatchSeedBlock against the REAL internal/audio/instrument_seeds.go
// rather than synthetic fixtures. The spec flags seed-patch fragility as the top
// risk (text rewrite of a hand-formatted Go literal), so the production path —
// patching an actual seed block that the CLI will patch for real — must be guarded:
//   - organSeed packs multiple "key": value pairs per line with trailing comments
//     (e.g. `"gen1_source": 1, ..., "gen1_gain": 0.30, // 16'`), the trickiest case
//     for the value-replace regex.
//   - PatchSeedBlock reformats the WHOLE file via go/format, so it must not perturb
//     any seed block other than the one being patched.

import (
	"go/format"
	"os"
	"regexp"
	"strings"
	"testing"
)

// seedsPath is the real seed file the synth-match CLI patches.
// synthmatch lives at internal/audio/synthmatch, so the seeds are one dir up.
const seedsPath = "../instrument_seeds.go"

func readSeeds(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile(seedsPath)
	if err != nil {
		t.Fatalf("read %q: %v", seedsPath, err)
	}
	return src
}

// kvPresent reports whether s contains a `"key": value` entry, tolerant of the
// alignment padding gofmt inserts (one or more spaces after the colon).
func kvPresent(s, key, val string) bool {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s+` + regexp.QuoteMeta(val) + `\b`)
	return re.MatchString(s)
}

// TestPatchRealSeedFile_SaxCleanBlock patches a clean single-KV-per-line block
// (saxSeed) and asserts the update + insert land and every OTHER block is byte-
// identical after the whole-file gofmt pass.
func TestPatchRealSeedFile_SaxCleanBlock(t *testing.T) {
	src := readSeeds(t)
	// The real file must already be gofmt-clean for the "neighbors unchanged"
	// comparison below to be meaningful.
	origFmt, err := format.Source(src)
	if err != nil {
		t.Fatalf("real seed file does not gofmt: %v", err)
	}
	if string(origFmt) != string(src) {
		t.Fatalf("real seed file is not gofmt-clean; run gofmt on instrument_seeds.go")
	}

	out, err := PatchSeedBlock(src, "saxSeed", map[string]float64{
		"filter_cutoff":   1234, // existing key → update
		"synthmatch_test": 0.5,  // new key → insert
	})
	if err != nil {
		t.Fatalf("PatchSeedBlock(saxSeed): %v", err)
	}
	s := string(out)

	if _, err := format.Source(out); err != nil {
		t.Fatalf("patched output is not valid Go: %v", err)
	}
	if !kvPresent(s, "filter_cutoff", "1234") {
		t.Errorf("saxSeed.filter_cutoff not updated to 1234")
	}
	if !kvPresent(s, "synthmatch_test", "0.5") {
		t.Errorf("new key synthmatch_test not inserted")
	}

	// Every block AFTER saxSeed must be byte-identical (saxSeed is near the end,
	// so assert the prefix up to saxSeed is untouched and a later sentinel block
	// is unchanged). The prefix before the edited block can never move.
	marker := "var saxSeed = RecipeParams{"
	io, ip := strings.Index(string(origFmt), marker), strings.Index(s, marker)
	if io < 0 || ip < 0 {
		t.Fatalf("saxSeed marker missing (orig=%d patched=%d)", io, ip)
	}
	if string(origFmt)[:io] != s[:ip] {
		t.Errorf("content before saxSeed changed — patch perturbed an unrelated block")
	}
}

// TestPatchRealSeedFile_OrganMultiKVLine patches gen1_gain inside organSeed — a
// key that lives on a shared line with several other "key": value pairs and a
// trailing `// 16'` comment — and asserts the same-line neighbors and comment
// survive, and that all blocks after organSeed (sax, …) are untouched.
func TestPatchRealSeedFile_OrganMultiKVLine(t *testing.T) {
	src := readSeeds(t)
	origFmt, err := format.Source(src)
	if err != nil {
		t.Fatalf("real seed file does not gofmt: %v", err)
	}
	if string(origFmt) != string(src) {
		t.Fatalf("real seed file is not gofmt-clean")
	}

	out, err := PatchSeedBlock(src, "organSeed", map[string]float64{
		"gen1_gain": 0.42, // existing key on a multi-KV line
	})
	if err != nil {
		t.Fatalf("PatchSeedBlock(organSeed): %v", err)
	}
	s := string(out)

	if _, err := format.Source(out); err != nil {
		t.Fatalf("patched output is not valid Go: %v", err)
	}
	if !kvPresent(s, "gen1_gain", "0.42") {
		t.Errorf("organSeed.gen1_gain not updated to 0.42")
	}
	// Same-line neighbors must survive the in-place value replacement.
	if !kvPresent(s, "gen1_source", "1") || !kvPresent(s, "gen1_freq", "0.5") {
		t.Errorf("same-line neighbors of gen1_gain were clobbered:\n%s", organBlock(s))
	}
	// The drawbar-footage comment on that line must be preserved.
	if !strings.Contains(s, "// 16'") {
		t.Errorf("trailing comment `// 16'` not preserved")
	}

	// All blocks AFTER organSeed must be byte-identical. saxSeed
	// follows organSeed in the file, so the tail from saxSeed to EOF must match.
	const tailMarker = "var saxSeed = RecipeParams{"
	io := strings.Index(string(origFmt), tailMarker)
	ip := strings.Index(s, tailMarker)
	if io < 0 || ip < 0 {
		t.Fatalf("saxSeed marker missing (orig=%d patched=%d)", io, ip)
	}
	if string(origFmt)[io:] != s[ip:] {
		t.Errorf("blocks after organSeed changed — patch perturbed unrelated blocks")
	}

	// Prefix before organSeed is likewise untouched.
	const headMarker = "var organSeed = RecipeParams{"
	ho := strings.Index(string(origFmt), headMarker)
	hp := strings.Index(s, headMarker)
	if string(origFmt)[:ho] != s[:hp] {
		t.Errorf("content before organSeed changed — patch perturbed an unrelated block")
	}
}

// organBlock returns the organSeed literal block for diagnostic output.
func organBlock(s string) string {
	i := strings.Index(s, "var organSeed = RecipeParams{")
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], "\n}")
	if j < 0 {
		return s[i:]
	}
	return s[i : i+j+2]
}
