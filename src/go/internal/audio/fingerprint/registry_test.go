package fingerprint

import "testing"

func TestRegistryRejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	d := Descriptor{ID: "x", Group: "g", Name: "n", Doc: "d",
		Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(1) }}
	r.Register(d)
	defer func() {
		if recover() == nil {
			t.Fatal("registering a duplicate ID must panic")
		}
	}()
	r.Register(d)
}

func TestRegistryRejectsNilExtract(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("registering a descriptor with a nil Extract must panic")
		}
	}()
	r.Register(Descriptor{ID: "x", Group: "g", Name: "n", Doc: "d", Applies: AlwaysApplies})
}

// No descriptor enters without an ID, group, name, documentation, an explicit applicability decision and an extractor.
func TestRegistryDisciplineEveryDescriptorIsComplete(t *testing.T) {
	for _, d := range DefaultRegistry().All() {
		if d.ID == "" {
			t.Errorf("descriptor with empty ID in group %q", d.Group)
			continue
		}
		if d.Group == "" {
			t.Errorf("descriptor %q: Group is required", d.ID)
		}
		if d.Name == "" {
			t.Errorf("descriptor %q: Name is required (the human-facing axis label)", d.ID)
		}
		if d.Doc == "" {
			t.Errorf("descriptor %q: Doc is required — say what it reads and why that source", d.ID)
		}
		if d.Extract == nil {
			t.Errorf("descriptor %q: Extract is required", d.ID)
		}
		if d.Applies == nil {
			t.Errorf("descriptor %q: Applies is required (use AlwaysApplies to say so explicitly)", d.ID)
		}
	}
}

func TestRegistryAllIsSortedAndStable(t *testing.T) {
	a := DefaultRegistry().All()
	b := DefaultRegistry().All()
	if len(a) != len(b) {
		t.Fatalf("All() length varies: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("All() order is not stable at %d: %q vs %q", i, a[i].ID, b[i].ID)
		}
	}
}

// Alphabetical order would read as a scrambled report.
func TestRegistryAllPreservesNarrativeGroupOrder(t *testing.T) {
	want := []string{GroupNoise, GroupEnvelope, GroupPartials, GroupBright, GroupMod}
	var got []string
	seen := map[string]bool{}
	for _, d := range DefaultRegistry().All() {
		if !seen[d.Group] {
			seen[d.Group] = true
			got = append(got, d.Group)
		}
	}
	// Only the core groups are pinned; later phases append further groups.
	if len(got) < len(want) {
		t.Fatalf("got %d groups, want at least %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("group %d: got %q want %q (full order: %v)", i, got[i], w, got)
		}
	}
}

func TestRegistrySortedByGroupIsCanonical(t *testing.T) {
	s := DefaultRegistry().SortedByGroup()
	for i := 1; i < len(s); i++ {
		prev, cur := s[i-1], s[i]
		if prev.Group > cur.Group || (prev.Group == cur.Group && prev.ID >= cur.ID) {
			t.Fatalf("SortedByGroup not sorted at %d: (%q,%q) then (%q,%q)",
				i, prev.Group, prev.ID, cur.Group, cur.ID)
		}
	}
}

func TestRegistryFilterByGroup(t *testing.T) {
	groups := DefaultRegistry().Groups()
	if len(groups) == 0 {
		t.Skip("no descriptors registered yet")
	}
	g := groups[0]
	got := DefaultRegistry().Filter(Filter{Group: &g})
	if len(got) == 0 {
		t.Fatalf("filtering by group %q returned nothing", g)
	}
	for _, d := range got {
		if d.Group != g {
			t.Errorf("filter leaked descriptor %q from group %q", d.ID, d.Group)
		}
	}
}

func TestApplicabilityPredicatesDiscriminate(t *testing.T) {
	tone := NewBundle(bundleTone(44100, 220, 2), "tone")
	if !RequiresAudible(tone) {
		t.Error("a steady tone must be audible")
	}
	if !RequiresPitched(tone) {
		t.Error("a steady tone must be pitched")
	}
	if RequiresPercussive(tone) {
		t.Error("a steady tone must not be classified percussive")
	}

	silent := NewBundle(bundleSilence(44100, 1), "silent")
	if RequiresAudible(silent) {
		t.Error("silence must not be audible")
	}
	if RequiresSustain(silent) {
		t.Error("silence must not be classified as sustaining")
	}
	if RequiresPercussive(silent) {
		t.Error("silence must not be classified percussive")
	}
}
