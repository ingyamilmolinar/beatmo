package log

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Tag prefixes demoted from INFO: INFO is for user actions and major lifecycle events, not mechanism. Re-introducing one
// means removing it here and justifying it in the PR.
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

// Scans internal/ui and internal/audio for an Infof whose format starts with a forbidden tag.
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
			// Tests may log fixture data at INFO with any tag.
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

// Independent of the runner's cwd (the package dir).
func repoRoot() (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	// here = .../internal/log/forbidden_at_info_test.go → walk up to src/go.
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..")), nil
}
