package log

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// forbiddenInfoPatterns lists tag prefixes that have been *demoted* from INFO
// to DEBUG/WARN as part of the logging signal/noise overhaul. If anyone
// re-introduces an Infof("[ZOOM]"...) (etc.), this test catches it before
// it lands.
//
// To intentionally re-introduce one of these tags at INFO, remove it from
// this list AND justify the change in the PR description (the level contract
// in package log says INFO is for user actions and major component lifecycle,
// not mechanism detail).
var forbiddenInfoPatterns = []string{
	"[ZOOM]",
	"[zoom]",
	"[PERF]",
	"[perf]",
	"[PAN-DEBUG]",
	"[PAN]",
	"[pan]",
	"[TIMELINE]",
	"[timeline]",
	"[REFRESH]",
	"[refresh]",
	"[UPDATE_BEAT_INFOS]",
	"[update_beat_infos]",
	"[BENCH_JSON]",
	"[bench_json]",
}

// TestForbiddenInfoPatterns scans internal/ui/ and internal/audio/ for
// Infof calls whose format string starts with a forbidden tag. Fails if any
// reappear so the demotion done in this overhaul does not silently regress.
func TestForbiddenInfoPatterns(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	dirs := []string{
		filepath.Join(root, "internal", "ui"),
		filepath.Join(root, "internal", "audio"),
		filepath.Join(root, "core"),
	}

	pat := regexp.MustCompile(`Infof\("(\[[^\]]+\])`)

	type hit struct {
		path string
		line int
		tag  string
		text string
	}
	var hits []hit

	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip test files — tests can legitimately log fixture data
			// at INFO with arbitrary tags.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				m := pat.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				tag := m[1]
				if !isForbidden(tag) {
					continue
				}
				hits = append(hits, hit{path: path, line: i + 1, tag: tag, text: strings.TrimSpace(line)})
			}
			return nil
		})
	}

	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		rel, _ := filepath.Rel(root, h.path)
		t.Errorf("forbidden INFO tag %s at %s:%d — demote to Debugf/Warnf or remove (see internal/log doc for the level contract):\n    %s",
			h.tag, rel, h.line, h.text)
	}
}

func isForbidden(tag string) bool {
	for _, p := range forbiddenInfoPatterns {
		if tag == p {
			return true
		}
	}
	return false
}

// repoRoot returns the absolute path to the src/go directory by walking up
// from this test file's location. Avoids depending on the test runner's cwd
// (which is the package dir).
func repoRoot() (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	// here = .../internal/log/forbidden_at_info_test.go → walk up to src/go.
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..")), nil
}
