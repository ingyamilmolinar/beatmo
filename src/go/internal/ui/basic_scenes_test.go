package ui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBasicScenesListValid guards scripts/basic_scenes.txt (the curated
// `make screenshots-basic` set) against catalog drift: every listed name
// must exist in the scene catalog, and the list must not carry duplicates.
func TestBasicScenesListValid(t *testing.T) {
	path := findRepoFile(t, filepath.Join("scripts", "basic_scenes.txt"))
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	known := make(map[string]bool, len(sceneCatalog))
	for _, s := range sceneCatalog {
		known[s.Name] = true
	}

	seen := map[string]bool{}
	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
		if seen[line] {
			t.Errorf("basic_scenes.txt: duplicate scene %q", line)
		}
		seen[line] = true
		if !known[line] {
			t.Errorf("basic_scenes.txt: scene %q not in scene catalog (renamed or removed?)", line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if count == 0 {
		t.Fatal("basic_scenes.txt: no scenes listed")
	}
}

// findRepoFile walks up from cwd until rel exists, mirroring findDesignMD.
func findRepoFile(t *testing.T, rel string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("%s not found walking up from %s", rel, cwd)
	return ""
}
