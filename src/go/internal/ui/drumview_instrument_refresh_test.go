package ui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestRefreshInstrumentsSkipsWhenUnchanged(t *testing.T) {
	withAudioCatalog(t, []audio.SoundMeta{{ID: "sample-1", Name: "Sample 1", Category: "Samples"}})
	dv := newTestDrumView(t, 800, 600)
	if dv.instMeta == nil || dv.instCatByID == nil {
		t.Fatalf("expected initial instrument metadata to be set")
	}
	metaPtr := reflect.ValueOf(dv.instMeta).Pointer()
	catPtr := reflect.ValueOf(dv.instCatByID).Pointer()
	opts := append([]string(nil), dv.instOptions...)

	dv.refreshInstruments()
	if reflect.ValueOf(dv.instMeta).Pointer() != metaPtr {
		t.Fatalf("expected refresh to skip meta rebuild when unchanged")
	}
	if reflect.ValueOf(dv.instCatByID).Pointer() != catPtr {
		t.Fatalf("expected refresh to skip category rebuild when unchanged")
	}
	if !slices.Equal(dv.instOptions, opts) {
		t.Fatalf("expected instrument options to remain stable when unchanged")
	}
}

func TestRefreshInstrumentsUpdatesOnCatalogChange(t *testing.T) {
	withAudioCatalog(t, []audio.SoundMeta{{ID: "old", Name: "Old", Category: "Olds"}})
	dv := newTestDrumView(t, 800, 600)
	if _, ok := dv.instMeta["old"]; !ok {
		t.Fatalf("expected initial catalog entry to exist")
	}
	audio.ResetCatalogForTest([]audio.SoundMeta{{ID: "new", Name: "New", Category: "News"}})
	dv.refreshInstruments()
	if _, ok := dv.instMeta["new"]; !ok {
		t.Fatalf("expected refreshed catalog entry to exist")
	}
	if _, ok := dv.instMeta["old"]; ok {
		t.Fatalf("expected old catalog entry to be replaced")
	}
}

func TestEnsureInstrumentKnownForcesRefresh(t *testing.T) {
	withAudioCatalog(t, nil)
	dv := newTestDrumView(t, 800, 600)
	missing := "missing-sample"
	if slices.Contains(dv.instOptions, missing) {
		t.Fatalf("missing instrument unexpectedly present before ensure")
	}
	dv.EnsureInstrumentKnown(missing)
	if !slices.Contains(dv.instOptions, missing) {
		t.Fatalf("missing instrument not surfaced after ensure")
	}
}

func TestRefreshInstrumentsUpdatesOnRegister(t *testing.T) {
	withAudioCatalog(t, nil)
	dv := newTestDrumView(t, 800, 600)
	id := "custom-inst"
	if slices.Contains(dv.instOptions, id) {
		t.Fatalf("custom instrument unexpectedly present before register")
	}
	audio.Register(id, nil)
	dv.refreshInstruments()
	if !slices.Contains(dv.instOptions, id) {
		t.Fatalf("custom instrument missing after register")
	}
}
