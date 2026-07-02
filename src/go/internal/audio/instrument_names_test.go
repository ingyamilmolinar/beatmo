package audio

import "testing"

func TestInstrumentDisplayNameResolution(t *testing.T) {
	ClearInstrumentDisplayName("snare")
	// Falls back to PrettyName(id) when no override and no catalog name.
	if got := InstrumentDisplayName("hi-hat-x"); got != "Hi Hat X" {
		t.Fatalf("pretty fallback: got %q want %q", got, "Hi Hat X")
	}
	// Override wins.
	SetInstrumentDisplayName("hi-hat-x", "My Hat")
	if got := InstrumentDisplayName("hi-hat-x"); got != "My Hat" {
		t.Fatalf("override: got %q want %q", got, "My Hat")
	}
	// Clearing restores the fallback.
	ClearInstrumentDisplayName("hi-hat-x")
	if got := InstrumentDisplayName("hi-hat-x"); got != "Hi Hat X" {
		t.Fatalf("after clear: got %q want %q", got, "Hi Hat X")
	}
	// Empty id yields empty.
	if got := InstrumentDisplayName(""); got != "" {
		t.Fatalf("empty id: got %q want empty", got)
	}
}

func TestInstrumentDisplayNameSettingEmptyClears(t *testing.T) {
	SetInstrumentDisplayName("foo-bar", "Custom")
	SetInstrumentDisplayName("foo-bar", "")
	if got := InstrumentDisplayName("foo-bar"); got != "Foo Bar" {
		t.Fatalf("setting empty should clear override: got %q", got)
	}
}

func TestInstrumentDisplayNameRelPathFallback(t *testing.T) {
	ResetCatalogForTest([]SoundMeta{
		{ID: "x", RelPath: "samples/hi-hat.wav"}, // no explicit Name
	})
	defer ResetCatalogForTest(nil)
	if got := InstrumentDisplayName("x"); got != "Hi Hat" {
		t.Fatalf("RelPath fallback: got %q want %q", got, "Hi Hat")
	}
	// Explicit catalog Name takes precedence over RelPath.
	ResetCatalogForTest([]SoundMeta{
		{ID: "y", Name: "Cowbell Deluxe", RelPath: "samples/whatever.wav"},
	})
	if got := InstrumentDisplayName("y"); got != "Cowbell Deluxe" {
		t.Fatalf("catalog Name precedence: got %q want %q", got, "Cowbell Deluxe")
	}
}
