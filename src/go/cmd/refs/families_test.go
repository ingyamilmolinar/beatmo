//go:build !js

package main

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Every ref instrument-tier and non-status, so it participates like a real catalogue entry.
func refsFrom(refs ...wavref.Ref) wavref.Cat {
	for i := range refs {
		refs[i].Tier = wavref.TierInstrument
	}
	return wavref.Cat{Refs: refs}
}

func measured(semis int) *wavref.Measured {
	return &wavref.Measured{SemisFromA3: semis, DurationSec: 1.5}
}

func TestBuildAuditionFamilies_InstrumentsArrayRoute(t *testing.T) {
	// oboe-A4 covers BOTH oboe-full and oboe in one ref's Instruments array
	// -- the two must land in the same family without any naming-convention
	// help (neither id is a hyphenated variant of the other's base).
	c := refsFrom(wavref.Ref{
		ID:          "oboe-A4",
		Instruments: []instrument.ID{roster.OboeFull, roster.Oboe},
		File:        "oboe_A4.wav",
		Measured:    measured(3),
	})

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("families = %d, want 1", len(families))
	}
	fam := families[0]
	if fam.Key != roster.Oboe {
		t.Errorf("Key = %q, want %q", fam.Key, "oboe")
	}
	if want := []instrument.ID{roster.Oboe, roster.OboeFull}; !reflect.DeepEqual(fam.Members, want) {
		t.Errorf("Members = %v, want %v", fam.Members, want)
	}
	if fam.Reference.ID != "oboe-A4" {
		t.Errorf("Reference.ID = %q, want %q", fam.Reference.ID, "oboe-A4")
	}
}

func TestBuildAuditionFamilies_NamingConventionRoute(t *testing.T) {
	// cello and cello-warm are covered by TWO DIFFERENT refs -- the only
	// thing that unions them is the shared "cello" prefix. The family must
	// pick ONE reference for both members (share-count tie -> first by id).
	c := refsFrom(
		wavref.Ref{ID: "cello-C2", Instruments: []instrument.ID{roster.Cello}, File: "cello_C2.wav", Measured: measured(-21)},
		wavref.Ref{ID: "cello-A3", Instruments: []instrument.ID{roster.CelloWarm}, File: "cello_A3.wav", Measured: measured(0)},
	)

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("families = %d, want 1: %+v", len(families), families)
	}
	fam := families[0]
	if fam.Key != roster.Cello {
		t.Errorf("Key = %q, want %q", fam.Key, "cello")
	}
	if want := []instrument.ID{roster.Cello, roster.CelloWarm}; !reflect.DeepEqual(fam.Members, want) {
		t.Errorf("Members = %v, want %v", fam.Members, want)
	}
	// Both refs cover exactly one member each (share count 1-1): tie broken
	// by ref id, and "cello-A3" < "cello-C2" lexicographically.
	if fam.Reference.ID != "cello-A3" {
		t.Errorf("Reference.ID = %q, want %q (tie-break: first by id)", fam.Reference.ID, "cello-A3")
	}
}

func TestBuildAuditionFamilies_ThreeMembersPreferReferenceCoveringMost(t *testing.T) {
	// conga-tumba joins by name with its own single-member ref; the family reference is the one most members share.
	c := refsFrom(
		wavref.Ref{ID: "conga-freesound-alt", Instruments: []instrument.ID{roster.Conga, roster.CongaOpen}, File: "conga_freesound.wav", Measured: measured(22)},
		wavref.Ref{ID: "conga-vcsl", Instruments: []instrument.ID{roster.CongaTumba}, File: "conga_vcsl.wav", Measured: measured(-17)},
	)

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("families = %d, want 1: %+v", len(families), families)
	}
	fam := families[0]
	if want := []instrument.ID{roster.Conga, roster.CongaOpen, roster.CongaTumba}; !reflect.DeepEqual(fam.Members, want) {
		t.Errorf("Members = %v, want %v", fam.Members, want)
	}
	if fam.Reference.ID != "conga-freesound-alt" {
		t.Errorf("Reference.ID = %q, want %q (covers 2 of 3 members)", fam.Reference.ID, "conga-freesound-alt")
	}
}

func TestBuildAuditionFamilies_NoSiblingYieldsSingleOptionFamily(t *testing.T) {
	c := refsFrom(wavref.Ref{
		ID:          "bassoon-A2",
		Instruments: []instrument.ID{roster.Bassoon},
		File:        "bassoon_A2.wav",
		Measured:    measured(-7),
	})

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("families = %d, want 1", len(families))
	}
	fam := families[0]
	if fam.Key != roster.Bassoon || len(fam.Members) != 1 || fam.Members[0] != roster.Bassoon {
		t.Errorf("fam = %+v, want single-member bassoon family", fam)
	}
	if fam.Reference.ID != "bassoon-A2" {
		t.Errorf("Reference.ID = %q, want %q", fam.Reference.ID, "bassoon-A2")
	}
}

func TestBuildAuditionFamilies_UnrelatedHyphenatedIdsStayApart(t *testing.T) {
	// dnb-kick must NOT be folded into any "kick" family just because it
	// contains a hyphen -- there is no covered "kick" base instrument here,
	// so naming-convention grouping must not fire.
	c := refsFrom(
		wavref.Ref{ID: "dnb-kick-ref", Instruments: []instrument.ID{roster.DnbKick}, File: "dnb.wav", Measured: measured(-10)},
		wavref.Ref{ID: "zgump-kick-ref", Instruments: []instrument.ID{roster.ZgumpKick}, File: "zgump.wav", Measured: measured(-12)},
	)

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 2 {
		t.Fatalf("families = %d, want 2 (dnb-kick and zgump-kick must stay separate): %+v", len(families), families)
	}
	for _, fam := range families {
		if len(fam.Members) != 1 {
			t.Errorf("family %q has %d members, want 1", fam.Key, len(fam.Members))
		}
	}
}

func TestBuildAuditionFamilies_SortedByKey(t *testing.T) {
	c := refsFrom(
		wavref.Ref{ID: "violin-D5", Instruments: []instrument.ID{roster.Violin}, File: "violin_D5.wav", Measured: measured(17)},
		wavref.Ref{ID: "bassoon-A2", Instruments: []instrument.ID{roster.Bassoon}, File: "bassoon_A2.wav", Measured: measured(-7)},
	)

	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 2 {
		t.Fatalf("families = %d, want 2", len(families))
	}
	if families[0].Key != roster.Bassoon || families[1].Key != roster.Violin {
		t.Errorf("families not sorted by Key: got [%s, %s]", families[0].Key, families[1].Key)
	}
}

func TestBuildAuditionFamilies_StatusRefsExcluded(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		{ID: "organ-status", Tier: wavref.TierInstrument, Instruments: []instrument.ID{roster.Organ}, Status: wavref.StatusNeedsSource, Note: "no source found"},
	}}
	families, err := buildAuditionFamilies(c)
	if err != nil {
		t.Fatalf("buildAuditionFamilies: %v", err)
	}
	if len(families) != 0 {
		t.Fatalf("families = %d, want 0 (status-only refs are not comparable)", len(families))
	}
}
