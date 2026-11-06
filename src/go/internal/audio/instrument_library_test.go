package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestShippedLibraryMatchesTheSources(t *testing.T) {
	src := loadShippedDocsForTest(t)
	ids := roster.All
	if len(ids) != len(src) || ids[0] != roster.All[0] {
		t.Fatalf("pack order %d ids, sources %d", len(ids), len(src))
	}
	for _, id := range ids {
		d, ok := ShippedDoc(id)
		if !ok || len(instrument.Diff(d, src[string(id)])) != 0 {
			t.Fatalf("%s: pack doc differs from its source", id)
		}
	}
}

// Every shipped document in the embedded pack validates against this build's
// registry (the loader skips an invalid one rather than crash at init).
func TestShippedPackValidates(t *testing.T) {
	shippedPack()
	if len(packInvalid) > 0 {
		t.Fatalf("the embedded pack holds documents this build rejects — run `make instrument-docs asset-pack`:\n%v", packInvalid)
	}
}

func TestNewUserInstrumentIDIsContentAddressed(t *testing.T) {
	d, _ := ShippedDoc(roster.KickSnap)
	a := NewUserInstrumentID("My Snap!", d)
	if a != NewUserInstrumentID("My Snap!", d) || len(a) != len("user.my-snap.")+6 || a[:13] != "user.my-snap." {
		t.Fatalf("id %q", a)
	}
	if NewUserInstrumentID("Other", d) == a {
		t.Fatal("the name is part of the content")
	}
}
