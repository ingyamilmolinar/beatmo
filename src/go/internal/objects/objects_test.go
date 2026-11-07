package objects

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

type body struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

func TestDigestIsContentAddressed(t *testing.T) {
	a := Digest(body{"x", 1})
	if !strings.HasPrefix(a, "sha256:") || len(a) != len("sha256:")+64 {
		t.Fatalf("digest %q is not sha256:<64 hex>", a)
	}
	if b := Digest(body{"x", 1}); b != a {
		t.Fatalf("same content, different digests: %s vs %s", a, b)
	}
	if c := Digest(body{"x", 1.0000000001}); c == a {
		t.Fatal("a changed value kept its digest")
	}
}

func TestKindDecodesStrictly(t *testing.T) {
	var k Kind
	if err := json.Unmarshal([]byte(`"instrument"`), &k); err != nil || k != KindInstrument {
		t.Fatalf("decode instrument: %v %v", k, err)
	}
	if err := json.Unmarshal([]byte(`"recipe"`), &k); err == nil {
		t.Fatal("an unknown kind decoded")
	}
}

func TestSaveAndMoveKeepEveryVersion(t *testing.T) {
	s := NewStore()
	v1 := s.Put(KindInstrument, body{"a", 1})
	v2 := s.Put(KindInstrument, body{"a", 2})
	v3 := s.Put(KindInstrument, body{"a", 3})
	for _, v := range []string{v1, v2, v3} {
		s.Save("kick", v)
	}
	if cur, _ := s.Current("kick"); cur != v3 {
		t.Fatalf("current = %s, want the last save", cur)
	}
	if err := s.Move("kick", v1); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Current("kick"); cur != v1 {
		t.Fatal("moving back did not move the pointer")
	}
	if got := s.Histories["kick"].Versions; !slices.Equal(got, []string{v1, v2, v3}) {
		t.Fatalf("moving back dropped versions: %v", got)
	}
	if err := s.Move("kick", v3); err != nil {
		t.Fatal("moving forward again:", err)
	}
	s.Move("kick", v1)
	v4 := s.Put(KindInstrument, body{"a", 4})
	s.Save("kick", v4)
	if got := s.Histories["kick"].Versions; !slices.Equal(got, []string{v1, v2, v3, v4}) {
		t.Fatalf("a save after moving back must append: %v", got)
	}
	if err := s.Move("kick", "sha256:nope"); err == nil {
		t.Fatal("moved to a version the history does not hold")
	}
}

func TestSavingTheCurrentVersionAgainIsANoOp(t *testing.T) {
	s := NewStore()
	v := s.Put(KindInstrument, body{"a", 1})
	s.Save("kick", v)
	s.Save("kick", v)
	if n := len(s.Histories["kick"].Versions); n != 1 {
		t.Fatalf("re-saving the current version appended: %d versions", n)
	}
}

func TestBaseIsTheCurrentUntilASave(t *testing.T) {
	s := NewStore()
	if _, ok := s.Current("kick"); ok {
		t.Fatal("an identity with no save has a current version")
	}
	v := s.Put(KindInstrument, body{"a", 1})
	s.Save("kick", v)
	s.MoveToBase("kick")
	if _, ok := s.Current("kick"); ok {
		t.Fatal("the pointer on the base still reports a saved version")
	}
	if got := s.Histories["kick"].Versions; len(got) != 1 {
		t.Fatal("moving to the base dropped versions")
	}
}

func TestPruneKeepsNewestCurrentAndReferenced(t *testing.T) {
	s := NewStore()
	var bundles, versions []string
	for i := range 5 {
		b := s.Put(KindBundle, body{"eq", float64(i)})
		bundles = append(bundles, b)
		v := s.Put(KindInstrument, body{"kick", float64(i)}, b)
		versions = append(versions, v)
		s.Save("kick", v)
	}
	s.Move("kick", versions[0]) // the oldest is current
	orphan := s.Put(KindBundle, body{"orphan", 0})
	pinned := s.Put(KindBundle, body{"pinned", 0})

	s.Prune(2, pinned)

	want := []string{versions[0], versions[3], versions[4]}
	if got := s.Histories["kick"].Versions; !slices.Equal(got, want) {
		t.Fatalf("kept %v, want the current plus the newest 2: %v", got, want)
	}
	if cur, _ := s.Current("kick"); cur != versions[0] {
		t.Fatal("pruning moved the current pointer")
	}
	for _, d := range []string{versions[0], bundles[0], versions[3], bundles[3], versions[4], bundles[4], pinned} {
		if _, ok := s.Get(d); !ok {
			t.Fatalf("pruned a kept or referenced object %s", d)
		}
	}
	for _, d := range []string{versions[1], bundles[1], versions[2], bundles[2], orphan} {
		if _, ok := s.Get(d); ok {
			t.Fatalf("kept an unreachable object %s", d)
		}
	}
}

func TestStoreRoundTripsThroughJSON(t *testing.T) {
	s := NewStore()
	b := s.Put(KindBundle, body{"eq", 1})
	v := s.Put(KindInstrument, body{"kick", 1}, b)
	s.Save("kick", v)
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Store
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(&back)
	if string(again) != string(raw) {
		t.Fatalf("round trip changed the store:\n%s\n%s", raw, again)
	}
	var got body
	if err := back.Body(v, &got); err != nil || got != (body{"kick", 1}) {
		t.Fatalf("body after round trip = %+v, %v", got, err)
	}
}

// Runs under GOOS=js too: a version id must not depend on platform float formatting.
func TestDigestIsTheSameOnEveryPlatform(t *testing.T) {
	v := struct {
		Values []float64          `json:"values"`
		Params map[string]float64 `json:"params"`
		Name   string             `json:"name"`
	}{
		Values: []float64{0.1, 1e-7, 1e21, 123456.789, -0.034329453106454272, 1.0 / 3},
		Params: map[string]float64{"z": 2.5, "a": -1, "m": 0},
		Name:   "kick ünicode",
	}
	const want = "sha256:4b4379af9de3a36462c385fb6f45e7cbbc2d470f6d32d05525bd37eef43f594b"
	if got := Digest(v); got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
}
