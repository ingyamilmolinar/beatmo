package audio

import "testing"

func TestCatalogVersionBumpsOnInitAndReset(t *testing.T) {
	withDefaultAudio(t)
	v0 := CatalogVersion()
	root := t.TempDir()
	if err := InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog failed: %v", err)
	}
	v1 := CatalogVersion()
	if v1 <= v0 {
		t.Fatalf("catalog version did not bump after init: before=%d after=%d", v0, v1)
	}
	ResetCatalogForTest(nil)
	v2 := CatalogVersion()
	if v2 <= v1 {
		t.Fatalf("catalog version did not bump after reset: before=%d after=%d", v1, v2)
	}
}

func TestInstrumentsVersionBumpsOnRegister(t *testing.T) {
	withDefaultAudio(t)
	v0 := InstrumentsVersion()
	Register("test-inst", nil)
	v1 := InstrumentsVersion()
	if v1 <= v0 {
		t.Fatalf("instrument version did not bump after register: before=%d after=%d", v0, v1)
	}
	Register("test-inst", nil)
	v2 := InstrumentsVersion()
	if v2 != v1 {
		t.Fatalf("instrument version bumped on duplicate register: before=%d after=%d", v1, v2)
	}
	ResetInstruments()
	v3 := InstrumentsVersion()
	if v3 <= v2 {
		t.Fatalf("instrument version did not bump after reset: before=%d after=%d", v2, v3)
	}
}
