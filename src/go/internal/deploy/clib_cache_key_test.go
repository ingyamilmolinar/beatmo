package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The hash is of the built archive, so sources, headers, flags and compiler
// version all reach the Go cache key through it.

func clibRoot(t *testing.T, archive []byte) string {
	t.Helper()
	dst := t.TempDir()
	for _, d := range []string{"scripts", "build", "src/go/internal/audio"} {
		if err := os.MkdirAll(filepath.Join(dst, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "clib-hash.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "scripts", "clib-hash.sh"), b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "build", "libdrums.a"), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func clibHash(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("bash", append([]string{filepath.Join(root, "scripts", "clib-hash.sh")}, append(args, root)...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("clib-hash.sh: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCLibHashFollowsContentNotMtime(t *testing.T) {
	root := clibRoot(t, []byte("!<arch>\nfirst"))
	h1 := clibHash(t, root)
	if len(h1) != 16 {
		t.Fatalf("hash %q, want 16 hex", h1)
	}
	lib := filepath.Join(root, "build", "libdrums.a")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(lib, future, future); err != nil {
		t.Fatal(err)
	}
	if h := clibHash(t, root); h != h1 {
		t.Errorf("an mtime-only change moved the hash %s → %s: a fresh checkout would miss every cache", h1, h)
	}
	if err := os.WriteFile(lib, []byte("!<arch>\nsecond"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h := clibHash(t, root); h == h1 {
		t.Error("a different archive left the hash unchanged: Go would link the stale libdrums.a")
	}
}

func TestCLibHashIsKeyedIntoTheCgoPackage(t *testing.T) {
	root := clibRoot(t, []byte("!<arch>\nx"))
	h := clibHash(t, root)
	clibHash(t, root, "--write")
	gen := filepath.Join(root, "src", "go", "internal", "audio", "clib_hash.gen.go")
	src := mustRead(t, gen)
	for _, want := range []string{"//go:build !test && !js", "package audio", "#cgo CFLAGS: -DBEATMO_CLIB_HASH=" + h, `import "C"`} {
		if !strings.Contains(src, want) {
			t.Errorf("clib_hash.gen.go lacks %q:\n%s", want, src)
		}
	}
	before, _ := os.Stat(gen)
	time.Sleep(10 * time.Millisecond)
	clibHash(t, root, "--write")
	after, _ := os.Stat(gen)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("--write rewrote an unchanged file")
	}
}

// Rebuilding the same objects at another time must give the same archive bytes,
// or every fresh checkout would get a new hash and a cold cache.
func TestCLibArchiveIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.c"), []byte("int a(void){return 1;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	archive := func(out string) []byte {
		run("cc", "-O2", "-c", "a.c", "-o", "a.o")
		future := time.Now().Add(time.Duration(len(out)) * time.Hour)
		_ = os.Chtimes(filepath.Join(dir, "a.o"), future, future)
		run("ar", "rcsD", out, "a.o")
		b, _ := os.ReadFile(filepath.Join(dir, out))
		return b
	}
	if !bytes.Equal(archive("one.a"), archive("second.a")) {
		t.Error("`ar rcsD` embedded a timestamp: the archive hash would change on every fresh checkout")
	}
}

func TestCLibRuleNeverCleansTheGoCache(t *testing.T) {
	if strings.Contains(mustRead(t, filepath.Join(repoRoot(t), "Makefile")), "clean -cache") {
		t.Error("the Makefile wipes the Go build cache; key the cgo package on the archive instead")
	}
}
