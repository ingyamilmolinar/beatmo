package synthmatch

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The shipped document is the only place an instrument's values live.

const organDocPath = "../../assets/instruments/organ-church.json"

func readOrganDoc(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(organDocPath)
	if err != nil {
		t.Fatalf("read %q: %v", organDocPath, err)
	}
	return b
}

func TestPatchInstrumentDocChangesOnlyTheTunedFields(t *testing.T) {
	src := readOrganDoc(t)
	out, err := PatchInstrumentDoc(src, map[string]float64{"filter_cutoff": 4321, "amp_decay": 0.5})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	before, _ := instrument.ParseFile(src)
	after, err := instrument.ParseFile(out)
	if err != nil {
		t.Fatalf("patched document does not parse: %v", err)
	}
	reg := audio.BuiltinModuleRegistry()
	if err := instrument.Validate(after, reg); err != nil {
		t.Fatalf("patched document is invalid: %v", err)
	}
	bv, av := instrument.FieldValues(before, reg), instrument.FieldValues(after, reg)
	if av["filter_cutoff"] != 4321 || av["amp_decay"] != 0.5 {
		t.Errorf("tuned fields not written: cutoff=%v decay=%v", av["filter_cutoff"], av["amp_decay"])
	}
	for k, v := range bv {
		if k == "filter_cutoff" || k == "amp_decay" {
			continue
		}
		if av[k] != v {
			t.Errorf("untouched field %s moved: %v -> %v", k, v, av[k])
		}
	}
	// A no-op patch is byte-identical: the file round-trips through MarshalFile.
	same, err := PatchInstrumentDoc(src, nil)
	if err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	if !bytes.Equal(same, src) {
		t.Errorf("an empty patch rewrote the shipped document")
	}
}

func TestPatchInstrumentDocRejectsAFieldTheDocCannotTake(t *testing.T) {
	_, err := PatchInstrumentDoc(readOrganDoc(t), map[string]float64{"no_such_field": 1})
	if err == nil || !errors.Is(err, instrument.ErrUnknownField) {
		t.Fatalf("unknown field: err=%v, want ErrUnknownField", err)
	}
}

func TestPatchInstrumentDocFileWritesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "organ-church.json")
	if err := os.WriteFile(path, readOrganDoc(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PatchInstrumentDocFile(path, map[string]float64{"filter_cutoff": 2000}); err != nil {
		t.Fatalf("patch file: %v", err)
	}
	b, _ := os.ReadFile(path)
	d, err := instrument.ParseFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if v := instrument.FieldValues(d, audio.BuiltinModuleRegistry())["filter_cutoff"]; v != 2000 {
		t.Errorf("filter_cutoff on disk = %v, want 2000", v)
	}
}

// Including the fields that sort before the layer's source.
func TestPatchInstrumentDocAddsALayer(t *testing.T) {
	src, err := os.ReadFile("../../assets/instruments/tubular-bells.json")
	if err != nil {
		t.Fatal(err)
	}
	tuned := map[string]float64{"gen9_source": 1, "gen9_freq": 3.4527, "gen9_gain": 0.14}
	out, err := PatchInstrumentDoc(src, tuned)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	d, _ := instrument.ParseFile(out)
	got := instrument.FieldValues(d, audio.BuiltinModuleRegistry())
	for k, want := range tuned {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
}
