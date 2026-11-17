package instrument

import (
	"slices"
	"strings"
	"testing"
)

func TestDiffNamesChangedPaths(t *testing.T) {
	r := testRegistry(t)
	a := minimalDoc()
	b, _ := SetValue(a, r, StageRef("core"), "core_g", 2)
	b, _ = AddStage(b, r, "a", true)
	b.Render.Seconds = 2
	got := Diff(a, b)
	want := []string{"render", "stages.a", "stages.core.values.core_g"}
	if !slices.Equal(got, want) {
		t.Fatalf("Diff = %v, want %v", got, want)
	}
	if len(Diff(a, a.Clone())) != 0 {
		t.Fatal("a clone differs from its original")
	}
}

func TestContentHashIgnoresIdentity(t *testing.T) {
	a := minimalDoc()
	b := a.Clone()
	b.ID, b.Origin = "other", "base"
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("ID/Origin must not change the content hash")
	}
	b.Stages["core"].Values["core_g"] = 2
	if ContentHash(a) == ContentHash(b) {
		t.Fatal("a value change must change the content hash")
	}
	if !strings.HasPrefix(ContentHash(a), "sha256:") || len(ContentHash(a)) != len("sha256:")+16 {
		t.Fatal(ContentHash(a))
	}
}

func TestBlockHashSeesEveryBit(t *testing.T) {
	a := []float64{0, 1, 2}
	b := []float64{0, 1, 2.0000000000000004}
	if BlockHash(a) == BlockHash(b) || BlockHash(a) != BlockHash([]float64{0, 1, 2}) {
		t.Fatal("BlockHash must be exact and deterministic")
	}
}

func TestBlockHashWithEqualsHashOfTheEditedCopy(t *testing.T) {
	a := []float64{0, 1, 2}
	if BlockHashWith(a, 1, 5) != BlockHash([]float64{0, 5, 2}) || a[1] != 1 {
		t.Fatal("BlockHashWith must equal BlockHash of the edited copy and leave the block alone")
	}
}
