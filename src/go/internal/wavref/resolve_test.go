package wavref

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleRoot_FindsGoMod(t *testing.T) {
	root, ok := ModuleRoot()
	if !ok {
		t.Fatal("ModuleRoot() not found — the go.mod walk failed")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("ModuleRoot() = %q but it holds no go.mod: %v", root, err)
	}
}

// Resolve reads the one reference-audio root, $BEATMO_WAV_REFS.
func TestResolve_ReadsTheRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "violin_A4.wav"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, tmp)

	got, ok := Resolve("violin-A4")
	if !ok {
		t.Fatal(`Resolve("violin-A4") not found`)
	}
	if want := filepath.Join(tmp, "violin_A4.wav"); got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

// There is no in-tree fallback: an id whose file is absent from the root
// does not resolve, even if a stray copy sits in the old in-tree corpus dir.
func TestResolve_HasNoInTreeFallback(t *testing.T) {
	t.Setenv(EnvVar, t.TempDir()) // empty root
	if got, ok := Resolve("violin-A4"); ok {
		t.Errorf("Resolve() = %q from an empty root; reference audio must only come from Root()", got)
	}
}

func TestResolve_UnknownIDNotOK(t *testing.T) {
	if _, ok := Resolve("no-such-ref"); ok {
		t.Error("Resolve() returned ok for an unknown id")
	}
}

// PCMHash operates on already-decoded samples (this package must not import
// internal/audio — see resolve.go's PCMHash doc comment for the import-cycle
// reason), so it is tested directly against synthetic sample slices.
func TestPCMHash_DeterministicAndContentSensitive(t *testing.T) {
	s1 := []float32{0.1, -0.2, 0.3, 0.0, -1.0}
	h1 := PCMHash(s1, 44100)
	h2 := PCMHash(append([]float32{}, s1...), 44100)
	if h1 != h2 {
		t.Errorf("PCMHash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("PCMHash = %q, want 64 hex chars", h1)
	}
}

func TestPCMHash_SampleChangeChangesHash(t *testing.T) {
	base := PCMHash([]float32{0.1, -0.2, 0.3}, 44100)
	changed := PCMHash([]float32{0.1, -0.2, 0.31}, 44100)
	if base == changed {
		t.Error("PCMHash did not change when a sample value changed")
	}
}

func TestPCMHash_RateChangeChangesHash(t *testing.T) {
	samples := []float32{0.1, -0.2, 0.3}
	h44100 := PCMHash(samples, 44100)
	h48000 := PCMHash(samples, 48000)
	if h44100 == h48000 {
		t.Error("PCMHash did not change when the sample rate changed")
	}
}

func TestPCMHash_LengthChangeChangesHash(t *testing.T) {
	base := PCMHash([]float32{0.1, -0.2, 0.3}, 44100)
	longer := PCMHash([]float32{0.1, -0.2, 0.3, 0.0}, 44100)
	if base == longer {
		t.Error("PCMHash did not change when a trailing zero sample was appended")
	}
}

func TestVerifyHash_UnknownIDErrors(t *testing.T) {
	if err := VerifyHash("no-such-ref", "deadbeef"); err == nil {
		t.Error("VerifyHash() returned nil for an unknown id")
	}
}

// The catalogue is mutable across the plan (Task 5 populated pcm_sha256 for
// every current ref; Task 7 will add status refs that carry none by
// construction). Hardcoding an id here would rot every time the data
// changes, so this searches for a live example of the "no recorded hash"
// state and skips when the catalogue currently has none.
func TestVerifyHash_NoRecordedHashVerifiesVacuously(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, r := range c.Refs {
		if r.Status == "" && r.PCMSHA256 == "" {
			id = r.ID
			break
		}
	}
	if id == "" {
		t.Skip("no catalogue ref currently has an empty pcm_sha256 (real refs) — every real ref has been hashed")
	}
	if err := VerifyHash(id, "anything-at-all"); err != nil {
		t.Errorf("VerifyHash(%q, ...) with no recorded hash should verify vacuously, got: %v", id, err)
	}
}

// This is the branch that actually matters today: a ref WITH a recorded
// hash must accept the matching hash and reject a wrong one. Unlike the
// vacuous-pass test above, this doesn't depend on any particular catalogue
// state beyond "at least one ref has a pcm_sha256" — true today and for the
// rest of the plan, so it can't rot the way the hardcoded-id version did.
func TestVerifyHash_RealRecordedHash_MatchAndMismatch(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	var target Ref
	for _, r := range c.Refs {
		if r.PCMSHA256 != "" {
			target = r
			break
		}
	}
	if target.ID == "" {
		t.Skip("no catalogue ref currently carries a pcm_sha256")
	}

	if err := VerifyHash(target.ID, target.PCMSHA256); err != nil {
		t.Errorf("VerifyHash(%q, <matching hash>) = %v, want nil", target.ID, err)
	}
	if err := VerifyHash(target.ID, "definitely-not-the-hash"); err == nil {
		t.Errorf("VerifyHash(%q, <wrong hash>) = nil, want an error", target.ID)
	}
}

// The public VerifyHash always vacuously passes today, because none of the
// 5 catalogue.json refs carry a pcm_sha256 yet (Task 5 populates it). The
// mismatch branch is exercised directly against verifyRefHash with a
// synthetic Ref instead of depending on that future data.
func TestVerifyHash_MismatchErrors(t *testing.T) {
	r := Ref{ID: "fake-ref", PCMSHA256: "aaaa"}
	if err := verifyRefHash(r, "bbbb"); err == nil {
		t.Error("verifyRefHash() accepted a mismatched hash")
	}
	if err := verifyRefHash(r, "aaaa"); err != nil {
		t.Errorf("verifyRefHash() rejected a matching hash: %v", err)
	}
}
