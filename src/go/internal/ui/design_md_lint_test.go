package ui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDesignMDLintSnapshot pins the set of warnings emitted by
// `npx @google/design.md@0.1.1 lint DESIGN.md` against a checked-in
// snapshot at testdata/design_md_lint.expected.json.
//
// The lint produces real positive findings for tokens whose runtime
// rendering is acceptable but whose Stitch model can't represent the
// rendering (icon-only buttons, outline-only synthetic components, the
// destructive-text-on-overlay context menu pattern). Each such warning
// is intentional and documented in DESIGN.md prose + the design-token
// memory; a snapshot keeps that set bounded so new warnings cannot creep
// in unnoticed and removed warnings cannot be silently re-introduced.
//
// The test:
//   - Skips when `npx` or network is unavailable (offline CI / sandbox).
//     This is intentional — the snapshot is normative, but verifying it
//     requires a network/tool dependency we don't want to make blocking.
//   - Compares only (path, severity) tuples, not message text, because
//     contrast ratios may shift by ±0.01 due to floating point and the
//     text would otherwise be brittle.
//   - Always fails on errors; warnings must match the snapshot exactly.
func TestDesignMDLintSnapshot(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available; skipping DESIGN.md lint snapshot check")
	}

	repoRoot := findRepoRootForLint(t)
	cmd := exec.Command("npx", "-y", "@google/design.md@0.1.1", "lint", "DESIGN.md")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		// npx failed to fetch, network blocked, etc. Treat as skip rather
		// than fail so offline runs stay green. The snapshot remains
		// normative — anyone running this locally with network gets the
		// real check.
		t.Skipf("npx lint unavailable: %v", err)
	}

	type finding struct {
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	}
	type lintReport struct {
		Findings []finding `json:"findings"`
		Summary  struct {
			Errors   int `json:"errors"`
			Warnings int `json:"warnings"`
			Infos    int `json:"infos"`
		} `json:"summary"`
	}
	var got lintReport
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse lint json: %v\nraw: %s", err, string(out))
	}

	if got.Summary.Errors > 0 {
		var errs []string
		for _, f := range got.Findings {
			if f.Severity == "error" {
				errs = append(errs, f.Path+": "+f.Message)
			}
		}
		t.Fatalf("DESIGN.md lint reports %d error(s):\n  %s", got.Summary.Errors, strings.Join(errs, "\n  "))
	}

	type expectedEntry struct {
		Path     string `json:"path"`
		Severity string `json:"severity"`
	}
	type expected struct {
		Warnings []expectedEntry `json:"warnings"`
	}
	uiDir := findUIDir(t)
	expectedPath := filepath.Join(uiDir, "testdata", "design_md_lint.expected.json")
	body, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read snapshot %s: %v", expectedPath, err)
	}
	var exp expected
	// Ignore unknown fields (the snapshot includes _doc / _rationale).
	dec := json.NewDecoder(strings.NewReader(string(body)))
	if err := dec.Decode(&exp); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}

	wantSet := map[string]string{}
	for _, w := range exp.Warnings {
		wantSet[w.Path] = w.Severity
	}
	gotSet := map[string]string{}
	for _, f := range got.Findings {
		if f.Severity == "warning" {
			gotSet[f.Path] = f.Severity
		}
	}

	var unexpected []string
	for path, sev := range gotSet {
		if want, ok := wantSet[path]; !ok || want != sev {
			unexpected = append(unexpected, path+" ("+sev+")")
		}
	}
	var missing []string
	for path, sev := range wantSet {
		if got, ok := gotSet[path]; !ok || got != sev {
			missing = append(missing, path+" ("+sev+")")
		}
	}

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Errorf("new lint warning(s) not in snapshot:\n  %s\n"+
			"If intentional, add to %s with a justification in the existing _rationale block.",
			strings.Join(unexpected, "\n  "), expectedPath)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("expected lint warning(s) no longer reported:\n  %s\n"+
			"If you fixed the underlying issue, remove the entry from %s and update the _rationale.",
			strings.Join(missing, "\n  "), expectedPath)
	}
}

// findRepoRootForLint walks up from the test cwd looking for DESIGN.md.
func findRepoRootForLint(t *testing.T) string {
	t.Helper()
	dir := findUIDir(t)
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "DESIGN.md")
		if _, err := os.Stat(candidate); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate DESIGN.md from ui dir")
	return ""
}
