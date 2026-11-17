//go:build test

package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Export / import with macro dials (spec §4, §5.5 fast path). A dial is a lens:
// its edits are synth-param writes that travel in instrument_docs like any knob
// edit, and NOTHING about the dial itself (position, definition) is ever
// written to the project file — a dial's position is read from the document.

// Turned dials round-trip: export → reset the instrument → import restores
// every driven param, and the re-export is byte-identical.
func TestSynthDialEditsRoundTripThroughTheProjectFile(t *testing.T) {
	g := newDialGame(t)
	inst := dialInst(g)
	untouched := audio.GetInstrumentParams(instrument.ID(inst))

	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -70)
	dragFrom(g, centre(dialRect(t, g, "length", "tail")), 50)
	edited := audio.GetInstrumentParams(instrument.ID(inst))
	if paramsEqual(untouched, edited) {
		t.Fatal("the dial drags changed no param — the round-trip would pass vacuously")
	}
	file := mustExport(t, g)

	audio.ResetInstrumentParams(instrument.ID(inst))
	if paramsEqual(audio.GetInstrumentParams(instrument.ID(inst)), edited) {
		t.Fatal("reset did not wipe the dial edits")
	}
	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	idleFrame(g)
	if got := audio.GetInstrumentParams(instrument.ID(inst)); !paramsEqual(got, edited) {
		t.Fatalf("import did not restore the dial edits:\n got  %v\n want %v", got, edited)
	}
	if again := mustExport(t, g); !bytes.Equal(again, file) {
		t.Fatal("re-export after import is not byte-identical")
	}
}

// Nothing about a dial reaches the file: viewing the root and opening +
// cancelling a dial's wheel leave the export byte-identical, and no key in the
// document mentions a dial.
func TestSynthDialStateNeverReachesTheProjectFile(t *testing.T) {
	g := newDialGame(t)
	untouched := mustExport(t, g)
	p := centre(dialRect(t, g, "brightness", "bright"))

	tapAtPoint(g, centre(dialRect(t, g, "length", "length")))
	keyTap(g, ebiten.KeyEscape)
	if got := mustExport(t, g); !bytes.Equal(got, untouched) {
		t.Fatal("opening and cancelling a dial's wheel changed the export")
	}

	dragFrom(g, p, -40)
	var doc any
	if err := json.Unmarshal(mustExport(t, g), &doc); err != nil {
		t.Fatal(err)
	}
	walkJSONKeys(doc, func(k string) {
		if strings.Contains(strings.ToLower(k), "dial") {
			t.Errorf("project JSON carries a dial key %q", k)
		}
	})
}

func walkJSONKeys(v any, fn func(string)) {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			fn(k)
			walkJSONKeys(c, fn)
		}
	case []any:
		for _, c := range t {
			walkJSONKeys(c, fn)
		}
	}
}
