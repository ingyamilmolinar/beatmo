package wavref

import (
	"os"
	"path/filepath"
	"testing"
)

// Root() never reports not-ok for an unset variable: a drop-in directory you must export a variable to find is not one.
func TestRoot_HonoursEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvVar, tmp)

	if got := Root(); got != tmp {
		t.Errorf("Root() = %q, want %q", got, tmp)
	}
}

func TestRoot_DefaultsUnderHome(t *testing.T) {
	t.Setenv(EnvVar, "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if want := filepath.Join(home, ".beatmo", "wav-refs"); Root() != want {
		t.Errorf("Root() = %q, want the default %q", Root(), want)
	}
}

// The drop-in dir is a SUBDIRECTORY of the one root, alongside the
// candidates/ compare/ probes/ working dirs that already live there. It is
// not a second tree with its own environment variable.
func TestSourcesDir_IsASubdirectoryOfTheOneRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvVar, tmp)

	if want := filepath.Join(tmp, "sources"); SourcesDir() != want {
		t.Errorf("SourcesDir() = %q, want %q", SourcesDir(), want)
	}
}

// Resolve only hands back audio from the root itself, so a raw drop-in is invisible until adopted.
func TestSourcesDir_IsNotWhereResolveLooks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvVar, tmp)
	if err := os.MkdirAll(SourcesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// A raw drop-in that happens to carry a catalogued ref's filename.
	if err := os.WriteFile(filepath.Join(SourcesDir(), "violin_A4.wav"), []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := Resolve("violin-A4")
	if ok && filepath.Dir(got) == SourcesDir() {
		t.Errorf("Resolve() returned the raw drop-in %q; only converted audio may resolve", got)
	}
}

// A bare filename resolves inside the drop-in dir; that is what lets
// `refs adopt -wav snare_hot.wav` work with no path typing.
func TestSourcePath_BareNameResolvesInsideSourcesDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvVar, tmp)
	if err := os.MkdirAll(SourcesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(SourcesDir(), "snare_hot.wav")
	if err := os.WriteFile(want, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := SourcePath("snare_hot.wav")
	if err != nil {
		t.Fatalf("SourcePath: %v", err)
	}
	if got != want {
		t.Errorf("SourcePath() = %q, want %q", got, want)
	}
}

// Anything that looks like a path is used as given -- the drop-in dir is a
// convenience, never a cage.
func TestSourcePath_ExplicitPathBypassesSourcesDir(t *testing.T) {
	t.Setenv(EnvVar, t.TempDir()) // deliberately does NOT hold the file
	other := t.TempDir()
	want := filepath.Join(other, "elsewhere.wav")
	if err := os.WriteFile(want, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := SourcePath(want)
	if err != nil {
		t.Fatalf("SourcePath: %v", err)
	}
	if got != want {
		t.Errorf("SourcePath() = %q, want the path as given %q", got, want)
	}
}

// A missing file is an error, not a silent empty string: adopting audio
// that isn't there is a mistake the caller has to see.
func TestSourcePath_MissingFileIsAnError(t *testing.T) {
	t.Setenv(EnvVar, t.TempDir())

	if _, err := SourcePath("nope.wav"); err == nil {
		t.Fatal("SourcePath() returned nil error for an absent file")
	}
}

// ListSources enumerates what is waiting to be adopted, so a bulk scan has
// one place to ask. A missing dir is zero files, not an error -- nobody has
// dropped anything yet.
func TestListSources_ReturnsDropInFilesAndTolueratesAMissingDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvVar, tmp)

	got, err := ListSources()
	if err != nil {
		t.Fatalf("ListSources on a missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSources on a missing dir = %v, want none", got)
	}

	if err := os.MkdirAll(filepath.Join(SourcesDir(), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"snare-2.wav", "kick.aiff", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(SourcesDir(), n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err = ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	// Audio only, sorted, no directories: a bulk adopt must not try to
	// hand a README to ffmpeg.
	want := []string{"kick.aiff", "snare-2.wav"}
	if len(got) != len(want) {
		t.Fatalf("ListSources = %v, want %v", got, want)
	}
	for i := range want {
		if filepath.Base(got[i]) != want[i] {
			t.Errorf("ListSources[%d] = %q, want %q", i, filepath.Base(got[i]), want[i])
		}
	}
}
