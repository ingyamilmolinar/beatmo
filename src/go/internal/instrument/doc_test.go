package instrument

import (
	"strings"
	"testing"
)

func sampleDoc() Doc {
	return Doc{
		ID: "test-doc", Name: "Test Doc", Category: "modular", Notes: "retuned 2026-10-02",
		Render: Render{Seconds: 0.6, Amplitude: 0.8},
		Stages: map[string]Stage{"core": {V: 1, On: true, Values: Values{"core_g": 1}}},
		Layers: []*Module{nil, {Type: "tone", V: 1, On: true, Values: Values{"gain": 0.5}}},
		FX:     []Module{{Type: "fx.filter", V: 1, On: true, Values: Values{"cutoff": 4000}}},
	}
}

func TestFileRoundTrip(t *testing.T) {
	in := sampleDoc()
	b, err := MarshalFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schema_version": "0.0.1"`) {
		t.Fatalf("file lacks the schema version:\n%s", b)
	}
	out, err := ParseFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(in, out); len(d) != 0 || out.ID != in.ID || out.Layers[0] != nil {
		t.Fatalf("round trip changed the document: %v", d)
	}
}

func TestParseFileRejectsUnknownField(t *testing.T) {
	_, err := ParseFile([]byte(`{"schema_version":"0.0.1","id":"x","bogus":1}`))
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("want an unknown-field error naming bogus, got %v", err)
	}
}

func TestParseFileRejectsOtherVersion(t *testing.T) {
	_, err := ParseFile([]byte(`{"schema_version":"9.9.9","id":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("want a version error naming 9.9.9, got %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	a := sampleDoc()
	b := a.Clone()
	b.Stages["core"].Values["core_g"] = 7
	b.Layers[1].Values["gain"] = 7
	b.FX[0].Values["cutoff"] = 7
	if a.Stages["core"].Values["core_g"] != 1 || a.Layers[1].Values["gain"] != 0.5 || a.FX[0].Values["cutoff"] != 4000 {
		t.Fatal("Clone shares value maps with the original")
	}
}
