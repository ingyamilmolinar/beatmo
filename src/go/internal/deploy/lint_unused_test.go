package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The `unused` linter sees one build configuration; a helper whose only
// callers build under `-tags test` or js is reported though it is used. The
// rule: dead only if unused in EVERY configuration that compiles its file.
func TestUnusedIsDeadOnlyInEveryConfigThatBuildsTheFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	issue := func(file string, line int, text string) map[string]any {
		return map[string]any{"FromLinter": "unused", "Text": text, "Pos": map[string]any{"Filename": "src/go/" + file, "Line": line}}
	}
	// a.go builds everywhere; b.go only without the test tag; w.go only on js.
	write("linux.files.json", []string{"a.go", "b.go"})
	write("linux-test.files.json", []string{"a.go"})
	write("js.files.json", []string{"a.go", "w.go"})
	write("linux.issues.json", map[string]any{"Issues": []any{
		issue("a.go", 1, "func usedByTests is unused"),
		issue("a.go", 2, "func dead is unused"),
		issue("b.go", 3, "func realOnlyDead is unused"),
	}})
	write("linux-test.issues.json", map[string]any{"Issues": []any{
		issue("a.go", 2, "func dead is unused"),
	}})
	write("js.issues.json", map[string]any{"Issues": []any{
		issue("a.go", 1, "func usedByTests is unused"),
		issue("a.go", 2, "func dead is unused"),
		issue("w.go", 4, "func wasmOnlyDead is unused"),
	}})
	cmd := exec.Command("python3", filepath.Join(repoRoot(t), "scripts", "ci", "lint-unused.py"), "--from", dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("dead symbols present but the script passed:\n%s", out)
	}
	got := string(out)
	for _, want := range []string{"a.go:2: func dead is unused", "b.go:3: func realOnlyDead is unused", "w.go:4: func wasmOnlyDead is unused"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "usedByTests") {
		t.Errorf("a symbol the test build uses was reported dead:\n%s", got)
	}
}
